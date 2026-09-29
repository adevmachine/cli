package credentials

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/mydevmachine/devmachine/internal/remote"
)

// DefaultEnvFile is where a workspace's own secret lands when `secrets set`
// is given no `--env-file`: a sourceable file inside the workspace's home,
// relative to it.
//
// It is a published contract, the same way EnvFile is for a machine
// credential: a shell package sources this exact path on login, so it works
// against a CLI released years after this one. It does not change.
const DefaultEnvFile = workspaceDir + "/env"

// backupSuffix names the copy `--env-file` delivery keeps the first time it
// edits a file that already existed. The default file is entirely
// devmachine's own, so nothing ever backs it up.
const backupSuffix = ".devmachine.bak"

// SafeWorkspacePath cleans a path given for `--env-file` and refuses one
// that would land outside the workspace's home.
func SafeWorkspacePath(rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("the file path is empty")
	}
	if path.IsAbs(rel) {
		return "", fmt.Errorf("%q is an absolute path: give one relative to the workspace's home", rel)
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%q reaches outside the workspace's home", rel)
	}
	return cleaned, nil
}

// MergeDotenv returns body with name set to value: an existing `name=` line
// is replaced in place, and everything else — comments, blank lines, every
// other variable — is kept byte for byte. A name that is not already there is
// appended.
//
// The value is quoted exactly as EnvBody quotes it: unconditionally, in
// single quotes, so a space, a dollar, a backtick or a `#` in the value comes
// back as it went in.
func MergeDotenv(body []byte, name, value string) []byte {
	line := name + "=" + shellQuote(value)
	prefix := name + "="

	lines := dotenvLines(body)
	found := false
	for i, l := range lines {
		if strings.HasPrefix(l, prefix) {
			lines[i] = line
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, line)
	}
	return joinDotenvLines(lines)
}

// removeDotenvLine drops the line that sets name, keeping everything else
// unchanged. changed is false when name was never there, so a caller never
// writes a file back unchanged.
func removeDotenvLine(body []byte, name string) (out []byte, changed bool) {
	prefix := name + "="
	lines := dotenvLines(body)

	kept := lines[:0]
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			changed = true
			continue
		}
		kept = append(kept, l)
	}
	if !changed {
		return body, false
	}
	return joinDotenvLines(kept), true
}

func dotenvLines(body []byte) []string {
	text := string(body)
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func joinDotenvLines(lines []string) []byte {
	if len(lines) == 0 {
		return []byte{}
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// readDotenv reads rel, relative to user's home on the machine c reaches,
// returning its content and whether it existed at all — an absent file and
// an empty one are not the same thing for a caller deciding whether to keep
// a backup.
func readDotenv(ctx context.Context, c remote.Client, user, rel string) (content []byte, existed bool, err error) {
	script := fmt.Sprintf(`set -eu
user=%s
rel=%s
home=$(getent passwd "$user" | cut -d: -f6)
if [ -z "$home" ]; then
	echo "there is no account named $user on this machine" >&2
	exit 1
fi
full="$home/$rel"
if [ -f "$full" ]; then
	printf 'EXISTS\n'
	cat "$full"
else
	printf 'MISSING\n'
fi
`, shellQuote(user), shellQuote(rel))

	out, err := c.Run(ctx, script)
	if err != nil {
		return nil, false, fmt.Errorf("reading %s: %w", rel, err)
	}
	marker, rest, _ := strings.Cut(out, "\n")
	switch marker {
	case "EXISTS":
		return []byte(rest), true, nil
	case "MISSING":
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("reading %s: unexpected response %q", rel, out)
	}
}

// dotenvWriteScript writes what arrives on stdin to rel, relative to user's
// home, atomically and owned by user. It never chmods a file that already
// existed — %[3]s decides whether a first edit keeps a backup, which only
// ever happens for a person's own file (`--env-file`): the default file is
// entirely devmachine's, and backing it up would just be noise.
const dotenvWriteScript = `set -eu
umask 077
user=%[1]s
rel=%[2]s
backup=%[3]s
home=$(getent passwd "$user" | cut -d: -f6)
if [ -z "$home" ]; then
	echo "there is no account named $user on this machine" >&2
	exit 1
fi
full="$home/$rel"
dir=$(dirname "$full")
make_dirs() {
	if [ -d "$1" ]; then
		return
	fi
	make_dirs "$(dirname "$1")"
	mkdir "$1"
	chmod 0700 "$1"
	chown "$user" "$1"
}
make_dirs "$dir"
chmod 0700 "$dir"
chown "$user" "$dir"
if [ "$backup" = "1" ] && [ -e "$full" ] && [ ! -e "$full` + backupSuffix + `" ]; then
	cp -p "$full" "$full` + backupSuffix + `"
	chown "$user" "$full` + backupSuffix + `"
fi
mode=0600
if [ -e "$full" ]; then
	mode=$(stat -c %%a "$full" 2>/dev/null || stat -f %%Lp "$full")
fi
tmp="$dir/.devmachine.$$"
: > "$tmp"
chmod "$mode" "$tmp"
chown "$user" "$tmp"
cat > "$tmp"
mv -f "$tmp" "$full"
`

func writeDotenv(ctx context.Context, c remote.Client, user, rel string, backup bool, content []byte) error {
	flag := "0"
	if backup {
		flag = "1"
	}
	script := fmt.Sprintf(dotenvWriteScript, shellQuote(user), shellQuote(rel), flag)
	if _, err := c.RunInput(ctx, script, bytes.NewReader(content)); err != nil {
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	return nil
}

// PushWorkspaceEnv delivers name=value into rel, a dotenv-style file inside
// the workspace's home — the default ~/.devmachine/env, or the file
// `--env-file` named. Everything already in rel is kept, other than the one
// line this sets.
//
// backup is only ever true for `--env-file`: see dotenvWriteScript.
func PushWorkspaceEnv(ctx context.Context, c remote.Client, user, rel, name, value string, backup bool) error {
	if value == "" {
		return fmt.Errorf("secret %q has no value to deliver", name)
	}
	original, _, err := readDotenv(ctx, c, user, rel)
	if err != nil {
		return err
	}
	return writeDotenv(ctx, c, user, rel, backup, MergeDotenv(original, name, value))
}

// RemoveWorkspaceEnv removes name from rel, leaving everything else as it
// was. Nothing is written when rel does not exist, or name was never in it.
func RemoveWorkspaceEnv(ctx context.Context, c remote.Client, user, rel, name string, backup bool) error {
	original, existed, err := readDotenv(ctx, c, user, rel)
	if err != nil {
		return err
	}
	if !existed {
		return nil
	}
	out, changed := removeDotenvLine(original, name)
	if !changed {
		return nil
	}
	return writeDotenv(ctx, c, user, rel, backup, out)
}
