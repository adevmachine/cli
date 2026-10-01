// Package upload sends local files into an account's home on a machine.
package upload

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/mydevmachine/devmachine/internal/remote"
	"golang.org/x/text/unicode/norm"
)

// DefaultDir is where a file lands when no folder is named, relative to the
// home of the account it is sent to.
const DefaultDir = ".cache/devmachine/uploads"

const timestampLayout = "20060102-150405"

var unsafeInName = strings.NewReplacer("/", "_", "\x00", "_", "\n", "_", "\r", "_")

// Name is the name a file gets on the machine, split so the machine can add
// a counter between the two when the name is taken: the original name with
// the time before its extension, so a second upload of report.pdf never
// replaces the first.
func Name(base string, at time.Time) (stem, ext string) {
	base = unsafeInName.Replace(base)
	stem = base
	if i := strings.LastIndex(base, "."); i > 0 && i < len(base)-1 {
		stem, ext = base[:i], base[i:]
	}
	return stem + "-" + at.Format(timestampLayout), ext
}

// Dir cleans the folder given for --dir. A relative folder, or one starting
// with `~/`, is relative to the home; an absolute one is kept for the
// machine to check, since only the machine knows where the home is.
func Dir(dir string) (string, error) {
	if dir == "" {
		return DefaultDir, nil
	}
	if strings.ContainsAny(dir, "\x00\n\r") {
		return "", fmt.Errorf("--dir %q has a line break or a NUL byte", dir)
	}
	if dir == "~" {
		return ".", nil
	}
	if strings.HasPrefix(dir, "~") && !strings.HasPrefix(dir, "~/") {
		return "", fmt.Errorf("--dir %q names another account's home: give a folder inside the target's own home", dir)
	}
	dir = strings.TrimPrefix(dir, "~/")
	if dir == "" {
		return ".", nil
	}
	if path.IsAbs(dir) {
		return path.Clean(dir), nil
	}
	cleaned := path.Clean(dir)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("--dir %q reaches outside the home", dir)
	}
	return cleaned, nil
}

var octalMode = regexp.MustCompile(`^0?[0-7]{3}$`)

// Mode checks the permissions given for --mode.
func Mode(mode string) (string, error) {
	if !octalMode.MatchString(mode) {
		return "", fmt.Errorf("--mode %q is not an octal mode such as 600 or 0644", mode)
	}
	return mode, nil
}

// Request is one file to place: in Dir, as Stem+Ext, or Stem-N+Ext when that
// name is taken.
type Request struct {
	Dir  string
	Stem string
	Ext  string
	Mode string
}

// script runs as the account the file is for, so everything it creates is
// that account's without a chown. Its arguments arrive base64-encoded: a
// file name is never shell syntax, whatever it holds.
//
// It has no single quote in it, since it travels inside one to whatever
// login shell the account has. The home check refuses a folder that leaves
// the home through a symbolic link, the same rule credentials delivery keeps.
// The folder also arrives composed (NFC), and that form is used when the one
// given does not exist yet: a Mac app's process arguments are decomposed
// (NFD), while a Linux folder name is almost always composed.
// The file is written to a temporary name and hard-linked into place, which
// fails rather than replaces when the name is taken, even against a second
// upload running at the same time.
const script = `set -eu
umask 077
fail() {
	printf "error %s\n" "$*"
	exit 1
}
decode() {
	printf %s "$1" | base64 -d
}
dir=$(decode "$1") || fail "cannot decode the folder"
stem=$(decode "$2") || fail "cannot decode the file name"
ext=$(decode "$3") || fail "cannot decode the file name"
mode=$4
home=${HOME:-}
[ -n "$home" ] || fail "the account has no home folder"
real_home=$(cd -P "$home" 2>/dev/null && pwd -P) || fail "the home $home is not a folder"
composed=$(decode "$5") || fail "cannot decode the folder"
under_home() {
	case "$1" in
	/*) printf %s "$1" ;;
	*) printf %s "$home/$1" ;;
	esac
}
full=$(under_home "$dir")
if [ ! -e "$full" ] && [ ! -L "$full" ]; then
	dir=$composed
	full=$(under_home "$dir")
fi
case "$full" in
"$home" | "$home"/* | "$real_home" | "$real_home"/*) ;;
*) fail "$dir is outside the home ($home)" ;;
esac
inside() {
	case "$1/" in
	"$real_home"/*) return 0 ;;
	esac
	return 1
}
probe=$full
while [ ! -e "$probe" ] && [ ! -L "$probe" ]; do
	probe=$(dirname "$probe")
done
real_probe=$(cd -P "$probe" 2>/dev/null && pwd -P) || fail "$dir is not a folder: $probe is in the way"
inside "$real_probe" || fail "$dir reaches outside the home through a symbolic link"
mkdir -p "$full" 2>/dev/null || fail "cannot create $dir"
real_dir=$(cd -P "$full" 2>/dev/null && pwd -P) || fail "$dir is not a folder"
inside "$real_dir" || fail "$dir reaches outside the home through a symbolic link"
tmp=$(mktemp "$real_dir/.devmachine-upload.XXXXXX") || fail "cannot write in $real_dir"
cleanup() {
	rm -f "$tmp"
}
trap cleanup EXIT
cat > "$tmp" || fail "the file did not arrive whole"
chmod "$mode" "$tmp"
name=$stem$ext
n=1
while ! ln "$tmp" "$real_dir/$name" 2>/dev/null; do
	if [ ! -e "$real_dir/$name" ] && [ ! -L "$real_dir/$name" ]; then
		fail "cannot place $name in $real_dir"
	fi
	n=$((n + 1))
	[ "$n" -le 1000 ] || fail "$real_dir already holds 1000 files named $stem$ext"
	name=$stem-$n$ext
done
printf "ok %s\n" "$real_dir/$name"
`

func command(r Request) string {
	arg := func(s string) string {
		return `"` + base64.StdEncoding.EncodeToString([]byte(s)) + `"`
	}
	return "sh -c '" + script + "' devmachine-upload " +
		arg(r.Dir) + " " + arg(r.Stem) + " " + arg(r.Ext) + " " + r.Mode + " " + arg(norm.NFC.String(r.Dir))
}

// Send streams body into place on the machine c reaches and returns the
// absolute path it landed at.
func Send(ctx context.Context, c remote.Client, r Request, body io.Reader) (string, error) {
	if _, err := Mode(r.Mode); err != nil {
		return "", err
	}
	out, err := c.RunInput(ctx, command(r), body)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if p, ok := strings.CutPrefix(line, "ok "); ok && err == nil {
			return p, nil
		}
		if msg, ok := strings.CutPrefix(line, "error "); ok {
			return "", errors.New(msg)
		}
	}
	if err != nil {
		if errors.Is(err, remote.ErrHostKeyRejected) {
			return "", err
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("sending %s%s: the upload stopped on the machine: %w", r.Stem, r.Ext, exit)
		}
		return "", fmt.Errorf("sending %s%s: %w", r.Stem, r.Ext, err)
	}
	return "", fmt.Errorf("sending %s%s: the machine gave no path back: %q", r.Stem, r.Ext, out)
}
