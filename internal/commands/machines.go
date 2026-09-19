package commands

import (
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/local"
	"github.com/spf13/cobra"
)

func newMachinesCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machines",
		Short: "The servers this configuration knows about",
	}
	cmd.AddCommand(
		newMachinesListCmd(opts),
		newMachinesCreateLocalCmd(opts),
		newMachinesStartCmd(),
		newMachinesStopCmd(),
		newMachinesDeleteLocalCmd(),
	)
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

func newMachinesCreateLocalCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "create-local <name>",
		Short: "Create a machine on this computer, as a bought server arrives",
		Long: "Create a machine on this computer, as a bought server arrives.\n\n" +
			"It needs Lima. The machine comes up with root reachable over SSH by " +
			"password and no key installed, which is where `devmachine setup` starts.\n\n" +
			"Nothing is written to the configuration: `setup` does that.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Progress belongs on stderr, so `--format json` leaves a
			// document on stdout and nothing else.
			m, err := local.Create(cmd.Context(), args[0], cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			return reportLocalMachine(cmd, opts, m)
		},
	}
}

func newMachinesStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start a machine on this computer",
		Long: "Start a machine on this computer.\n\n" +
			"Only a local machine, created with `create-local`: a bought server " +
			"is not the CLI's to switch on.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := local.Start(cmd.Context(), args[0]); err != nil {
				return err
			}
			cmd.Printf("%s is running\n", args[0])
			return nil
		},
	}
}

func newMachinesStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a machine on this computer",
		Long: "Stop a machine on this computer, keeping its disk.\n\n" +
			"Only a local machine, created with `create-local`: a bought server " +
			"is not the CLI's to switch off.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := local.Stop(cmd.Context(), args[0]); err != nil {
				return err
			}
			cmd.Printf("%s is stopped\n", args[0])
			return nil
		},
	}
}

func newMachinesDeleteLocalCmd() *cobra.Command {
	var yes bool

	c := &cobra.Command{
		Use:   "delete-local <name>",
		Short: "Destroy a machine on this computer",
		Long: "Destroy a machine on this computer, and everything on it.\n\n" +
			"Only a local machine, created with `create-local`. It is not " +
			"`machines rm`, which forgets a server and leaves it running.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(),
					"Destroy the local machine "+args[0]+" and everything on it?")
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}
			if err := local.Delete(cmd.Context(), args[0]); err != nil {
				return err
			}
			cmd.Printf("%s is gone\n", args[0])
			return nil
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "destroy it without asking")
	return c
}

// reportLocalMachine prints a new local machine the way `machines list` prints
// a configured one, so the same fields mean the same thing.
func reportLocalMachine(cmd *cobra.Command, opts *options, m config.Machine) error {
	addresses := make([]string, 0, len(m.Hosts))
	for _, h := range m.Hosts {
		addresses = append(addresses, h.Address)
	}

	if opts.format == formatJSON {
		return writeJSON(cmd.OutOrStdout(), machineJSON{
			Name: m.Name, Hosts: addresses, AdminUser: m.User,
			Port: m.Port, Key: m.Key, Workspaces: []string{},
		})
	}

	cmd.Printf("%-12s %-28s port %-6d admin: %s\n",
		m.Name, strings.Join(addresses, ","), m.Port, m.User)
	cmd.Printf("It has no key on it yet, and the root password is %q.\n", local.Password)
	cmd.Printf("Take it over with `devmachine setup`.\n")
	return nil
}
