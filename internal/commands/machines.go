package commands

import (
	"strings"

	"github.com/spf13/cobra"
)

func newMachinesCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machines",
		Short: "The servers this configuration knows about",
	}
	cmd.AddCommand(newMachinesListCmd(opts))
	return cmd
}

func newMachinesListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the configured machines and the workspaces on each",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), asJSON(cfg).Machines)
			}

			for _, m := range cfg.Machines {
				addresses := make([]string, 0, len(m.Hosts))
				for _, h := range m.Hosts {
					addresses = append(addresses, h.Address)
				}
				names := make([]string, 0)
				for _, w := range cfg.WorkspacesOn(m.Name) {
					names = append(names, w.Name)
				}
				workspaces := "none"
				if len(names) > 0 {
					workspaces = strings.Join(names, ", ")
				}
				cmd.Printf("%-12s %-28s port %-6d workspaces: %s\n",
					m.Name, strings.Join(addresses, ","), m.Port, workspaces)
			}
			return nil
		},
	}
}
