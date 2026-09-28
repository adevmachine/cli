package remote

import (
	"strconv"
	"strings"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/hostkeys"
)

// StrictSSHArgs is the one host-identity policy for every process that hands
// a command to the system ssh binary: an interactive session (`ssh`,
// `mosh`), and a multiplexed one (`run`, through DialMux). The preflight
// validates the store first. If a caller skips it, an unreadable pin
// produces an unusable algorithm rather than silently widening trust.
func StrictSSHArgs(m config.Machine) []string {
	algorithms := "none"
	if store, err := hostkeys.Open(m.KnownHostsFile); err == nil {
		if key, err := store.Key(m.Name, m.Port); err == nil {
			algorithms = strings.Join(hostkeys.Algorithms(key), ",")
		}
	}
	args := []string{
		"-p", strconv.Itoa(m.Port),
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + m.KnownHostsFile,
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "HostKeyAlias=" + hostkeys.Lookup(m.Name, m.Port),
		"-o", "UpdateHostKeys=no",
		"-o", "CheckHostIP=no",
		"-o", "VerifyHostKeyDNS=no",
		"-o", "KnownHostsCommand=none",
		"-o", "HostKeyAlgorithms=" + algorithms,
	}
	if m.Key != "" {
		args = append(args, "-i", m.Key, "-o", "IdentitiesOnly=yes")
	}
	return args
}
