package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/hostkeys"
	"github.com/adevmachine/cli/internal/repo"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/knownhosts"
)

type trustResult struct {
	Machine              string `json:"machine"`
	Address              string `json:"address"`
	Status               string `json:"status"`
	KeyType              string `json:"key_type"`
	CurrentFingerprint   string `json:"current_fingerprint,omitempty"`
	PresentedFingerprint string `json:"presented_fingerprint"`
	Check                bool   `json:"check"`
	Changed              bool   `json:"changed"`
}

type trustOptions struct {
	check   bool
	replace bool
	yes     bool
}

func newMachinesTrustCmd(opts *options) *cobra.Command {
	var flags trustOptions
	cmd := &cobra.Command{
		Use:   "trust [name]",
		Short: "Inspect or deliberately update a machine's SSH host key",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			name, err := trustMachineName(args, opts.machine)
			if err != nil {
				return err
			}
			return runMachinesTrust(cmd.Context(), dir, name, cmd.InOrStdin(), cmd.OutOrStdout(), opts.format, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.check, "check", false, "compare the presented key without writing")
	cmd.Flags().BoolVar(&flags.replace, "replace", false, "allow a deliberately changed host key to be replaced")
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "update the local trust store without asking")
	return cmd
}

func trustMachineName(args []string, flag string) (string, error) {
	if len(args) == 1 && flag != "" && args[0] != flag {
		return "", fmt.Errorf("the positional name %q and --machine %q select different machines", args[0], flag)
	}
	if len(args) == 1 {
		return args[0], nil
	}
	return flag, nil
}

func runMachinesTrust(ctx context.Context, dir, name string, in io.Reader, out io.Writer, format string, flags trustOptions) error {
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	machine, err := cfg.Machine(name)
	if err != nil {
		return err
	}
	if err := requiresAddress(machine, "machines trust"); err != nil {
		return err
	}
	presented, address, err := scanHostKey(ctx, machine)
	if err != nil {
		return err
	}
	store, err := hostkeys.Open(machine.KnownHostsFile)
	if err != nil {
		return err
	}

	result := trustResult{
		Machine:              machine.Name,
		Address:              address,
		KeyType:              presented.Type(),
		PresentedFingerprint: hostkeys.Fingerprint(presented),
		Check:                flags.check,
	}
	current, keyErr := store.Key(machine.Name, machine.Port)
	switch {
	case keyErr == nil && bytes.Equal(current.Marshal(), presented.Marshal()):
		result.Status = "matching"
		result.CurrentFingerprint = hostkeys.Fingerprint(current)
		return reportTrust(out, format, result)

	case keyErr == nil:
		result.Status = "changed"
		result.CurrentFingerprint = hostkeys.Fingerprint(current)
		if !flags.replace {
			return fmt.Errorf("host key changed for machine %q at %s: trusted %s, presented %s; this can mean an attack, a wrong address, or a deliberate rebuild; verify it before using --replace",
				machine.Name, address, result.CurrentFingerprint, result.PresentedFingerprint)
		}
		if flags.check {
			return reportTrust(out, format, result)
		}
		if !flags.yes {
			ok, err := confirmHostKey(in, out, fmt.Sprintf("Replace the trusted host key for %s from %s to %s?", machine.Name, result.CurrentFingerprint, result.PresentedFingerprint))
			if err != nil {
				return err
			}
			if !ok {
				return errDeclined
			}
		}
		if err := store.Put(machine.Name, machine.Port, presented); err != nil {
			return err
		}
		result.Changed = true
		repo.AutoCommit(ctx, dir, "chore(config): replace host key for "+machine.Name)
		return reportTrust(out, format, result)

	default:
		var unknown *knownhosts.KeyError
		if !errors.As(keyErr, &unknown) || len(unknown.Want) != 0 {
			return keyErr
		}
		result.Status = "missing"
		if flags.check {
			return reportTrust(out, format, result)
		}
		if !flags.yes {
			ok, err := confirmHostKey(in, out, fmt.Sprintf("Trust %s host key %s for %s? Compare it through another trusted channel first.", presented.Type(), result.PresentedFingerprint, machine.Name))
			if err != nil {
				return err
			}
			if !ok {
				return errDeclined
			}
		}
		if err := store.Put(machine.Name, machine.Port, presented); err != nil {
			return err
		}
		result.Changed = true
		repo.AutoCommit(ctx, dir, "chore(config): trust host key for "+machine.Name)
		return reportTrust(out, format, result)
	}
}

func reportTrust(out io.Writer, format string, result trustResult) error {
	if format == formatJSON {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "%s at %s presented %s %s\n", result.Machine, result.Address, result.KeyType, result.PresentedFingerprint)
	switch result.Status {
	case "matching":
		fmt.Fprintln(out, "the presented host key already matches the trusted key; nothing changed")
	case "missing":
		if result.Check {
			fmt.Fprintln(out, "no host key is trusted; --check wrote nothing")
		} else {
			fmt.Fprintln(out, "trusted the host key")
		}
	case "changed":
		fmt.Fprintf(out, "previous trusted fingerprint: %s\n", result.CurrentFingerprint)
		if result.Check {
			fmt.Fprintln(out, "the host key changed; --check wrote nothing")
		} else {
			fmt.Fprintln(out, "replaced the trusted host key")
		}
	}
	return nil
}
