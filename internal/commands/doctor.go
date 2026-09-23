package commands

import (
	"errors"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/credentials"
	"github.com/adevmachine/cli/internal/doctor"
	"github.com/spf13/cobra"
)

func newDoctorCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check whether the CLI can reach and operate the machine",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}

			// A configuration or a package set this cannot read is not a
			// reason to report nothing at all: doctor's first check is what
			// says so, in words, and a credential nobody could resolve is
			// simply not reported.
			var wanted []credentials.Declared
			if found, err := credentialsOnMachine(cmd.Context(), opts); err == nil {
				wanted = found.wanted
			}

			checks := doctor.RunWithScanner(cmd.Context(), dir, opts.machine, dial, scanHostKey, wanted)

			if opts.format == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), struct {
					Checks []doctor.Check `json:"checks"`
					OK     bool           `json:"ok"`
				}{checks, doctor.OK(checks)}); err != nil {
					return err
				}
			} else {
				width := 18
				for _, c := range checks {
					width = max(width, len(c.Name))
				}
				for _, c := range checks {
					cmd.Printf("%-4s  %-*s  %s\n", c.Status, width, c.Name, c.Detail)
				}
			}

			// A failing check is a failure of the machine, not of the command,
			// but exit 1 is what lets a script or an agent act on it.
			if !doctor.OK(checks) {
				return errors.New("some checks failed")
			}
			return nil
		},
	}
}
