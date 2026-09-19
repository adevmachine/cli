package credentials

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
)

// workspaceDir is where a workspace keeps the credentials delivered to it,
// inside its own home. The machine's own live in /etc/devmachine.
const workspaceDir = ".devmachine"

// Destination is the path on the machine a value is delivered to. A leading
// `~/` is the workspace's home, and only the machine knows where that is.
func Destination(d Declared) string {
	if isEnvDelivery(d) {
		if d.Workspace == "" {
			return EnvFile(d.Name)
		}
		return path.Join("~", workspaceDir, d.Name, "env")
	}
	return d.Path
}

// isEnvDelivery says whether this credential is delivered as a sourceable
// file rather than as the file the package named.
func isEnvDelivery(d Declared) bool {
	return d.Kind == packages.KindSecret && d.Env != ""
}

// EnvBody is the file a value is delivered into, and it is read by sourcing
// it: `set -a; . file; set +a`.
//
// The value is single-quoted, so a space, a dollar or a backtick in a token
// comes back as it went in. A single quote inside the value ends the quoting,
// escapes itself, and opens it again.
func EnvBody(d Declared, value string) string {
	return "# Written by the devmachine CLI. Sourced, never edited by hand.\n" +
		d.Env + "=" + shellQuote(value) + "\n"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// writeScript puts one value on the machine.
//
// The value arrives on stdin, never in an argument: an argument is in `ps` for
// every account on the machine to read while the command runs.
//
// Every directory it creates on the way is owned by the account the credential
// belongs to. Root making ~/.config on the way to a credential would otherwise
// leave a root-owned directory in a workspace's home, and the workspace's own
// tools would start failing for a reason nobody would connect to this. Neither
// `mkdir -p` nor `install -d -o` does it: GNU install gives the parents it
// creates the default owner and mode, and applies -o and -m to the last
// component alone.
const writeScript = `set -eu
umask 077
user=%s
path=%s
if [ -n "$user" ]; then
	home=$(getent passwd "$user" | cut -d: -f6)
	if [ -z "$home" ]; then
		echo "there is no account named $user on this machine" >&2
		exit 1
	fi
else
	home="$HOME"
	user=root
fi
case "$path" in
'~/'*) path="$home/${path#'~/'}" ;;
esac
make_dirs() {
	if [ -d "$1" ]; then
		return
	fi
	make_dirs "$(dirname "$1")"
	mkdir "$1"
	chmod 0700 "$1"
	chown "$user" "$1"
}
dir=$(dirname "$path")
make_dirs "$dir"
chmod 0700 "$dir"
chown "$user" "$dir"
: > "$path"
chmod 0600 "$path"
chown "$user" "$path"
cat > "$path"
`

// Push delivers one value to where the package that asked for it said it goes.
//
// It never prints the value, and never puts it in a command.
func Push(ctx context.Context, c remote.Client, d Declared, value string) error {
	if d.Kind == packages.KindLogin {
		return fmt.Errorf("credential %q is a login, and a login cannot be pushed: run `%s`",
			Key(d), LoginCommand(d))
	}
	if value == "" {
		return fmt.Errorf("credential %q has no value to deliver", Key(d))
	}
	destination := Destination(d)
	if destination == "" {
		return fmt.Errorf("package %q declares credential %q with neither `env` nor `path`, "+
			"so there is nowhere to deliver it", d.Package, d.Name)
	}

	body := value
	if isEnvDelivery(d) {
		body = EnvBody(d, value)
	}

	script := fmt.Sprintf(writeScript, shellQuote(d.LinuxUser), shellQuote(destination))
	if _, err := c.RunInput(ctx, script, strings.NewReader(body)); err != nil {
		return fmt.Errorf("delivering credential %q: %w", Key(d), err)
	}
	return nil
}

// LoginCommand is what a person runs to obtain a credential nobody can push.
func LoginCommand(d Declared) string {
	if d.Workspace == "" {
		return "devmachine login " + d.Name
	}
	return "devmachine login " + d.Name + " --workspace " + d.Workspace
}
