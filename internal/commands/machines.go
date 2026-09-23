package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/local"
	"github.com/adevmachine/cli/internal/repo"
	"github.com/spf13/cobra"
)

func newMachinesCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machines",
		Short: "The servers this configuration knows about",
	}
	cmd.AddCommand(
		newMachinesListCmd(opts),
		newMachinesAddCmd(opts),
		newMachinesTrustCmd(opts),
		newMachinesRmCmd(opts),
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

func newMachinesAddCmd(opts *options) *cobra.Command {
	var s setupOptions

	c := &cobra.Command{
		Use:   "add",
		Short: "Take over another machine and add it to the configuration",
		Long: "Asks the same questions as `setup`, minus the domain, and runs the " +
			"same bootstrap: it installs a key, proves the key on a connection of " +
			"its own, turns password login off, and installs Ansible.\n\n" +
			"`setup` writes the first machine. This writes every one after it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			return runMachinesAdd(cmd.Context(), dir, cmd.InOrStdin(), cmd.OutOrStdout(), s)
		},
	}
	c.Flags().BoolVar(&s.noHarden, "no-harden", false,
		"leave password login on (the key is still installed and proved)")
	return c
}

func newMachinesRmCmd(opts *options) *cobra.Command {
	var yes bool

	c := &cobra.Command{
		Use:   "rm <name>",
		Short: "Forget a machine, leaving the server running",
		Long: "Takes the machine out of the configuration and does nothing at all " +
			"to the server: it keeps running, with everything on it, and it is " +
			"still reachable by the key.\n\n" +
			"It is not `machines delete-local`, which destroys a machine on this " +
			"computer and everything on it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(),
					"Forget the machine "+args[0]+"? The server keeps running, untouched.")
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}
			if err := config.RemoveMachine(dir, args[0]); err != nil {
				return err
			}
			repo.AutoCommit(cmd.Context(), dir, "chore(config): remove machine "+args[0])
			cmd.Printf("%s is out of the configuration.\n", args[0])
			cmd.Printf("The server itself is untouched and still running: nothing on it was " +
				"changed or deleted, and the key still gets in.\n")
			cmd.Printf("`machines delete-local` is the one that destroys a machine, and only " +
				"one on this computer.\n")
			return nil
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "forget it without asking")
	return c
}

// runMachinesAdd asks for a machine, records it, then takes it over.
//
// It reads and writes through the streams it is given, for the same reason
// setup does: every branch of the bootstrap is reachable without a terminal.
func runMachinesAdd(ctx context.Context, dir string, in io.Reader, out io.Writer, opts setupOptions) error {
	current, err := config.Load(dir)
	if err != nil {
		return err
	}

	r := bufio.NewReader(in)
	m, err := askForMachine(r, out, "")
	if err != nil {
		return err
	}
	if m.Name == "" {
		return errors.New("a machine needs a name: it is how every other command says which one to act on")
	}
	if _, err := current.Machine(m.Name); err == nil {
		return fmt.Errorf("a machine named %q is already configured: pick another name", m.Name)
	}
	m.KnownHostsFile = filepath.Join(dir, config.KnownHostsFileName)
	if err := trustFirstContact(ctx, r, out, m); err != nil {
		return err
	}

	key, err := askForKey(r, out, dir, m.Name)
	if err != nil {
		return err
	}
	m.Key = key.Path

	if err := config.AddMachine(dir, m); err != nil {
		return err
	}
	repo.AutoCommit(ctx, dir, "chore(config): add machine "+m.Name)
	fmt.Fprintf(out, "\nadded %s to %s\n\n", m.Name, filepath.Join(dir, config.FileName))

	if err := bootstrap(ctx, r, in, out, m, key, opts.noHarden); err != nil {
		return err
	}

	fmt.Fprintf(out, "\nNext: `devmachine doctor --machine %s`, then `devmachine sync --machine %s`.\n",
		m.Name, m.Name)
	return nil
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
			"Only a local machine, created with `create-local`. This is the one " +
			"that destroys: `machines rm` only forgets a server, and leaves it " +
			"running.",
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
