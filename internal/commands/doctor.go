package commands

import (
	"errors"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/doctor"
	"github.com/adevmachine/cli/internal/remote"
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

			checks := doctor.Run(cmd.Context(), dir, remote.Dial)

			if opts.format == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), struct {
					Checks []doctor.Check `json:"checks"`
					OK     bool           `json:"ok"`
				}{checks, doctor.OK(checks)}); err != nil {
					return err
				}
			} else {
				for _, c := range checks {
					cmd.Printf("%-4s  %-18s  %s\n", c.Status, c.Name, c.Detail)
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
