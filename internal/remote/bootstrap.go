package remote

import (
	"context"
	"fmt"
	"io"
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
	client, err := ProveAuth(ctx, m, user, Auth{KeyPath: keyPath})
	if err != nil {
		return err
	}
	return client.Close()
}

// ProveAuth is ProveKey for a key nobody can point at: one an SSH agent holds
// and never writes to disk.
//
// It hands back the connection it proved, so everything that follows — turning
// password login off included — runs over the one that was proved rather than
// over the password session that is about to stop working.
func ProveAuth(ctx context.Context, m config.Machine, user string, a Auth) (Client, error) {
	client, _, err := DialWith(ctx, m, user, a)
	if err != nil {
		return nil, fmt.Errorf("%s is installed but does not log in as %q on machine %q. "+
			"Check that ~/.ssh/authorized_keys holds the line, that it is mode 0600 inside a 0700 ~/.ssh, "+
			"that sshd's AuthorizedKeysFile points at that file, and that PubkeyAuthentication is on: %w",
			a.describe(), user, m.Name, err)
	}
	return client, nil
}

// hardeningDropInPath is where password login is turned off.
//
// The 00- prefix is the whole trick, and the reason travels with it in the
// file's own first lines.
const hardeningDropInPath = "/etc/ssh/sshd_config.d/00-devmachine-hardening.conf"

// hardeningDropIn is the file's body.
const hardeningDropIn = `# Written by the devmachine CLI.
#
# 00- so it sorts first among the drop-ins. sshd uses the first value it finds
# for each directive, and the Include of sshd_config.d sits at the top of the
# main sshd_config, so whatever comes first in the alphabetical glob is in
# charge. An image that ships 60-cloudimg-settings.conf with
# PasswordAuthentication yes is why a 99- prefix silently does nothing: it is
# read last, and by then the answer is already decided.
PasswordAuthentication no
# Without this a password can still come back through PAM, and "password login
# is off" would not be true.
KbdInteractiveAuthentication no
PermitRootLogin prohibit-password
`

// writeDropInScript puts the body on disk. It arrives on stdin, so nothing in
// the file has to survive a round trip through shell quoting.
const writeDropInScript = `set -eu
umask 022
mkdir -p /etc/ssh/sshd_config.d
cat > ` + hardeningDropInPath + `
chmod 0644 ` + hardeningDropInPath + `
`

// validateScript asks sshd whether it would accept the configuration.
//
// sshd lives in sbin, which a non-interactive SSH session does not always have
// on its PATH.
const validateScript = `set -eu
PATH="$PATH:/usr/sbin:/sbin"
export PATH
sshd -t
`

// reloadScript picks the unit name the distribution uses: Debian and Ubuntu
// call it ssh, everybody else calls it sshd.
const reloadScript = `set -eu
if systemctl reload ssh 2>/dev/null; then
	exit 0
fi
systemctl reload sshd
`

// Harden turns password login off, and validates before reloading.
//
// A configuration sshd refuses plus a reload is a machine nobody can reach
// again, so validation is not a courtesy: it is the only thing between a typo
// and a rebuild. It runs after the key has been proved, never before.
func Harden(ctx context.Context, c Client) error {
	if _, err := c.RunInput(ctx, writeDropInScript, strings.NewReader(hardeningDropIn)); err != nil {
		return fmt.Errorf("writing %s: %w", hardeningDropInPath, err)
	}

	if out, err := c.Run(ctx, validateScript); err != nil {
		// A file the daemon refused must not stay: the next reload by
		// anything at all, a reboot included, would fail on it.
		if _, rmErr := c.Run(ctx, "rm -f "+hardeningDropInPath); rmErr != nil {
			return fmt.Errorf("sshd refused %s (%w) and it could not be taken away again (%w): "+
				"remove it by hand before sshd is reloaded", hardeningDropInPath, err, rmErr)
		}
		return fmt.Errorf("sshd refused the configuration, so password login is still on: %w: %s",
			err, strings.TrimSpace(out))
	}

	if _, err := c.Run(ctx, reloadScript); err != nil {
		return fmt.Errorf("reloading sshd: %w", err)
	}
	return nil
}

// OSReleaseCommand reads the file that says which distribution a machine is.
const OSReleaseCommand = "cat /etc/os-release"

// OSReleaseID reads ID from an os-release file. ID_LIKE is deliberately not
// consulted: "like debian" is a family, not a promise that a package of the
// same name exists.
func OSReleaseID(body string) string {
	for _, line := range strings.Split(body, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "ID=")
		if !ok {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"`)
	}
	return "unknown"
}

// aptInstallAnsible installs Ansible on a Debian or an Ubuntu.
//
// It installs `ansible`, not `ansible-core`: the core package carries no
// community.general, the firewall package needs that collection, and the
// failure only shows up much later, inside a play. This was found on a real
// machine, not reasoned about.
const aptInstallAnsible = `set -eu
if command -v ansible-playbook >/dev/null 2>&1; then
	echo "ansible is already installed"
	exit 0
fi
export DEBIAN_FRONTEND=noninteractive
# A cloud image that booted a minute ago is usually still running
# unattended-upgrades, which holds the dpkg lock. Without the timeout a first
# run loses that race and fails for a reason nobody can see.
apt-get -o DPkg::Lock::Timeout=300 update
apt-get -o DPkg::Lock::Timeout=300 install -y ansible
`

// ansibleInstall maps a distribution to the way Ansible is installed on it.
//
// It holds what has been run on a real machine and nothing else. A table with
// four entries nobody has tried gets somebody halfway through a first run and
// then leaves them there; an honest refusal that names their distribution at
// least tells them what to do next.
var ansibleInstall = map[string]string{
	"debian": aptInstallAnsible,
	"ubuntu": aptInstallAnsible,
}

// InstallAnsible puts Ansible on the machine.
//
// It is the last thing done by hand. Everything after it is a play, which is
// why this is the whole imperative surface and not the start of one.
//
// The output goes to out as it arrives: installing Ansible is minutes of work,
// and minutes of silence look like a machine that has stopped answering.
func InstallAnsible(ctx context.Context, c Client, out io.Writer) error {
	release, err := c.Run(ctx, OSReleaseCommand)
	if err != nil {
		return fmt.Errorf("reading /etc/os-release to find out which distribution this is: %w", err)
	}

	id := OSReleaseID(release)
	script, ok := ansibleInstall[id]
	if !ok {
		return fmt.Errorf("this CLI does not know how to install Ansible on %q: it has been run on "+
			"debian and ubuntu only. Install the `ansible` package by hand — not `ansible-core`, "+
			"which leaves out community.general — and run this again", id)
	}

	if err := c.Stream(ctx, script, out, out); err != nil {
		return fmt.Errorf("installing Ansible on %s: %w", id, err)
	}
	return nil
}
