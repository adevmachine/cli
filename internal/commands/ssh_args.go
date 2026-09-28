package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mydevmachine/cli/internal/config"
	"github.com/mydevmachine/cli/internal/hostkeys"
	"github.com/mydevmachine/cli/internal/remote"
	"golang.org/x/crypto/ssh/knownhosts"
)

// strictSSHArgs delegates to remote.StrictSSHArgs, the one host-identity
// policy shared with the multiplexed client run(1) uses.
func strictSSHArgs(m config.Machine) []string {
	return remote.StrictSSHArgs(m)
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
