package commands

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/hostkeys"
	"github.com/adevmachine/cli/internal/remote"
	"golang.org/x/crypto/ssh/knownhosts"
)

// strictSSHArgs is the one host-identity policy for every system SSH process.
// The preflight validates the store first. If a caller skips it, an unreadable
// pin produces an unusable algorithm rather than silently widening trust.
func strictSSHArgs(m config.Machine) []string {
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

func strictSSHCommand(m config.Machine) string {
	parts := []string{"ssh"}
	for _, arg := range strictSSHArgs(m) {
		parts = append(parts, quoteForShell(arg))
	}
	return strings.Join(parts, " ")
}

var verifySystemHost = realVerifySystemHost

func realVerifySystemHost(ctx context.Context, m config.Machine) error {
	presented, address, err := scanHostKey(ctx, m)
	if err != nil {
		return err
	}
	store, err := hostkeys.Open(m.KnownHostsFile)
	if err != nil {
		return err
	}
	pinned, err := store.Key(m.Name, m.Port)
	if err != nil {
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
			return fmt.Errorf("%w for machine %q: run `devmachine machines trust %s`", remote.ErrHostKeyUnknown, m.Name, m.Name)
		}
		return err
	}
	if err := store.Check(m.Name, m.Port, presented); err != nil {
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			return fmt.Errorf("%w for machine %q at %s: expected %s, received %s; verify the address, then run `devmachine machines trust %s --replace` only for a deliberate rebuild",
				remote.ErrHostKeyChanged, m.Name, address, hostkeys.Fingerprint(pinned), hostkeys.Fingerprint(presented), m.Name)
		}
		return err
	}
	return nil
}
