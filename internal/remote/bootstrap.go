package remote

import (
	"context"
	"fmt"
	"strings"

	"github.com/adevmachine/cli/internal/config"
)

// installKeyScript adds one line to the admin account's authorized_keys.
//
// The key arrives on stdin rather than in the command, because an argument is
// in `ps` for every account on the machine to read while the command runs. A
// public key is not a secret; the habit is what carries over to the values
// that are.
const installKeyScript = `set -eu
umask 077
dir="$HOME/.ssh"
file="$dir/authorized_keys"
mkdir -p "$dir"
chmod 0700 "$dir"
touch "$file"
chmod 0600 "$file"
key=$(cat)
if printf '%s\n' "$key" | grep -qxFf - "$file"; then
	exit 0
fi
if [ -s "$file" ] && [ -n "$(tail -c 1 "$file")" ]; then
	printf '\n' >> "$file"
fi
printf '%s\n' "$key" >> "$file"
`

// InstallKey appends a public key to the admin account's authorized_keys.
//
// Running it twice leaves one line: setup is run again on a machine it already
// owns more often than it is run on a new one.
func InstallKey(ctx context.Context, c Client, publicKey string) error {
	key := strings.TrimSpace(publicKey)
	if key == "" {
		return fmt.Errorf("no public key to install")
	}
	if _, err := c.RunInput(ctx, installKeyScript, strings.NewReader(key+"\n")); err != nil {
		return fmt.Errorf("installing the public key: %w", err)
	}
	return nil
}

// ProveKey opens a new connection using only the key, and returns an error
// describing what to do when it fails.
//
// The session that installed the key cannot prove anything about it: that
// session was authenticated by a password. A wrong mode on authorized_keys, an
// AuthorizedKeysFile pointing elsewhere, or SELinux all let the key install and
// still refuse it.
func ProveKey(ctx context.Context, m config.Machine, user, keyPath string) error {
	client, _, err := DialWith(ctx, m, user, Auth{KeyPath: keyPath})
	if err != nil {
		return fmt.Errorf("the key %s is installed but does not log in as %q on machine %q. "+
			"Check that ~/.ssh/authorized_keys holds the line, that it is mode 0600 inside a 0700 ~/.ssh, "+
			"that sshd's AuthorizedKeysFile points at that file, and that PubkeyAuthentication is on: %w",
			keyPath, user, m.Name, err)
	}
	defer client.Close()
	return nil
}
