package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/credentials"
	"github.com/mydevmachine/devmachine/internal/doctor"
	"github.com/spf13/cobra"
)

type machineReport struct {
	Machine string         `json:"machine"`
	Checks  []doctor.Check `json:"checks"`
	OK      bool           `json:"ok"`
}

func newDoctorCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check whether the CLI can reach and operate every machine, or the one --machine names",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}

			var pinned string
			var machines []config.Machine
			if cfg, err := loadConfig(opts); err == nil {
				pinned = cfg.Packages
				machines = cfg.Machines
			}

			if opts.machine != "" || len(machines) < 2 {
				return doctorOne(cmd, opts, dir, pinned)
			}
			return doctorEvery(cmd, opts, dir, pinned, machines)
		},
	}
}

// doctorOne checks the machine --machine names, or the only one there is.
func doctorOne(cmd *cobra.Command, opts *options, dir, pinned string) error {
	checks := machineChecks(cmd.Context(), opts, dir, opts.machine)
	checks = append(checks, freshnessChecks(cmd.Context(), pinned)...)

	if opts.format == formatJSON {
		if err := writeJSON(cmd.OutOrStdout(), struct {
			Checks []doctor.Check `json:"checks"`
			OK     bool           `json:"ok"`
		}{checks, doctor.OK(checks)}); err != nil {
			return err
		}
	} else {
		printChecks(cmd.OutOrStdout(), "", checks)
	}

	// A failing check is a failure of the machine, not of the command,
	// but exit 1 is what lets a script or an agent act on it.
	if !doctor.OK(checks) {
		return errors.New("some checks failed")
	}
	return nil
}

// doctorEvery checks every configured machine, one after the other.
//
// Doctor only reads, so with several machines and no --machine there is
// nothing to guess wrong: checking all of them is the answer to "is anything
// broken?". The checks about this computer run once, after the machines.
func doctorEvery(cmd *cobra.Command, opts *options, dir, pinned string, machines []config.Machine) error {
	var reports []machineReport
	var failed []string
	for _, m := range machines {
		checks := machineChecks(cmd.Context(), opts, dir, m.Name)
		ok := doctor.OK(checks)
		if !ok {
			failed = append(failed, m.Name)
		}
		reports = append(reports, machineReport{Machine: m.Name, Checks: checks, OK: ok})
	}
	local := freshnessChecks(cmd.Context(), pinned)
	ok := len(failed) == 0 && doctor.OK(local)

	if opts.format == formatJSON {
		if err := writeJSON(cmd.OutOrStdout(), struct {
			Machines []machineReport `json:"machines"`
			Checks   []doctor.Check  `json:"checks"`
			OK       bool            `json:"ok"`
		}{reports, local, ok}); err != nil {
			return err
		}
	} else {
		out := cmd.OutOrStdout()
		for _, r := range reports {
			fmt.Fprintf(out, "machine %s:\n", r.Machine)
			printChecks(out, "  ", r.Checks)
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, "this computer:")
		printChecks(out, "  ", local)
	}

	switch {
	case len(failed) > 0:
		return fmt.Errorf("some checks failed on %s", strings.Join(failed, ", "))
	case !ok:
		return errors.New("some checks failed on this computer")
	}
	return nil
}

// machineChecks runs doctor's checks on one machine; empty means the only one.
func machineChecks(ctx context.Context, opts *options, dir, machine string) []doctor.Check {
	machineOpts := *opts
	machineOpts.machine = machine

	// A configuration or a package set this cannot read is not a
	// reason to report nothing at all: doctor's first check is what
	// says so, in words, and a credential nobody could resolve is
	// simply not reported.
	var wanted []credentials.Declared
	if found, err := credentialsOnMachine(ctx, &machineOpts); err == nil {
		wanted = found.wanted
	}
	return doctor.RunWithScanner(ctx, dir, machine, dial, scanHostKey, wanted)
}

func printChecks(out io.Writer, indent string, checks []doctor.Check) {
	width := 18
	for _, c := range checks {
		width = max(width, len(c.Name))
	}
	for _, c := range checks {
		fmt.Fprintf(out, "%s%-4s  %-*s  %s\n", indent, c.Status, width, c.Name, c.Detail)
	}
}
