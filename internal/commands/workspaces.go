package commands

import (
	"fmt"
	"slices"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/keys"
	"github.com/spf13/cobra"
)

func newWorkspacesCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspaces",
		Short: "The environments this configuration knows about",
	}
	cmd.AddCommand(
		newWorkspacesListCmd(opts),
		newWorkspacesNewCmd(opts),
		newWorkspacesRmCmd(opts),
	)
	return cmd
}

// workspaceRow is the output contract, kept apart from the configuration
// struct so a change to how config.yml is written does not silently change
// what every consumer reads.
type workspaceRow struct {
	Name     string         `json:"name"`
	Machine  string         `json:"machine"`
	User     string         `json:"user"`
	Packages []string       `json:"packages"`
	Settings map[string]any `json:"settings,omitempty"`
}

func newWorkspacesListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Each workspace, the machine it lives on and what it gets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}

			rows := make([]workspaceRow, 0, len(cfg.Workspaces))
			for _, w := range cfg.Workspaces {
				rows = append(rows, workspaceRow{
					Name: w.Name, Machine: machineNameOf(cfg, w), User: w.LinuxUser(),
					Packages: w.Packages, Settings: w.Settings,
				})
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), struct {
					Workspaces []workspaceRow `json:"workspaces"`
				}{rows})
			}

			if len(rows) == 0 {
				cmd.Println("No workspace is configured. `devmachine workspaces new <name>` makes one.")
				return nil
			}
			cmd.Printf("%-16s %-12s %-12s %s\n", "NAME", "MACHINE", "USER", "PACKAGES")
			for _, row := range rows {
				list := "none"
				if len(row.Packages) > 0 {
					list = strings.Join(row.Packages, ", ")
				}
				cmd.Printf("%-16s %-12s %-12s %s\n", row.Name, row.Machine, row.User, list)
			}
			return nil
		},
	}
}

// machineNameOf names the machine a workspace runs on, filling in the only one
// there is when the entry leaves it out.
func machineNameOf(cfg config.Config, w config.Workspace) string {
	if w.Machine == "" && len(cfg.Machines) == 1 {
		return cfg.Machines[0].Name
	}
	return w.Machine
}

func newWorkspacesNewCmd(opts *options) *cobra.Command {
	var (
		like     string
		user     string
		packages []string
		check    bool
		yes      bool
	)

	c := &cobra.Command{
		Use:   "new <name>",
		Short: "Declare a workspace, for the next sync to create",
		Long: "It edits the configuration and touches no machine. `devmachine " +
			"sync` is what creates the Linux account.\n\n" +
			"The package list comes from `defaults.workspace` in the " +
			"configuration, unless --like or --packages says otherwise.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceNew(cmd, opts, args[0], workspaceNewOptions{
				like: like, user: user,
				packages: packages, packagesGiven: cmd.Flags().Changed("packages"),
				check: check, yes: yes,
			})
		},
	}
	c.Flags().StringVar(&like, "like", "", "copy another workspace's packages")
	c.Flags().StringVar(&user, "user", "", "the Linux account, when it cannot be the workspace's name")
	c.Flags().StringSliceVar(&packages, "packages", nil, "the packages it gets, instead of the default")
	c.Flags().BoolVar(&check, "check", false, "say what would change, and change nothing")
	c.Flags().BoolVar(&yes, "yes", false, "do not ask")
	return c
}

type workspaceNewOptions struct {
	like          string
	user          string
	packages      []string
	packagesGiven bool
	check         bool
	yes           bool
}

func runWorkspaceNew(cmd *cobra.Command, opts *options, name string, o workspaceNewOptions) error {
	dir, _, err := config.Dir(opts.configDir)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}
	if _, err := cfg.Workspace(name); err == nil {
		return fmt.Errorf("a workspace named %q is already configured: pick another name", name)
	}

	machine, err := cfg.Machine(opts.machine)
	if err != nil {
		return err
	}
	if err := adminKey(machine); err != nil {
		return err
	}

	list, err := packagesForNewWorkspace(cfg, o)
	if err != nil {
		return err
	}

	w := config.Workspace{Name: name, Machine: machine.Name, User: o.user, Packages: list}

	if o.check {
		cmd.Printf("would add the workspace %s to %s, with %s\n", name, machine.Name, describePackages(list))
		cmd.Println("Nothing was written.")
		return nil
	}
	if !o.yes {
		ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(),
			fmt.Sprintf("Add the workspace %s to %s, with %s?", name, machine.Name, describePackages(list)))
		if err != nil {
			return err
		}
		if !ok {
			return errDeclined
		}
	}

	if err := config.AddWorkspace(dir, w); err != nil {
		return err
	}
	cmd.Printf("%s is in the configuration, on %s, with %s.\n", name, machine.Name, describePackages(list))
	cmd.Println("The machine is untouched: `devmachine sync` is what creates the account.")
	return nil
}

// packagesForNewWorkspace decides what a new workspace gets.
//
// --packages beats --like, which beats the configured default. --like copies
// the packages and nothing else: a copied account name would collide, and a
// copied machine would put one workspace wherever another happens to be.
func packagesForNewWorkspace(cfg config.Config, o workspaceNewOptions) ([]string, error) {
	if o.packagesGiven {
		return o.packages, nil
	}
	if o.like != "" {
		source, err := cfg.Workspace(o.like)
		if err != nil {
			return nil, err
		}
		return slices.Clone(source.Packages), nil
	}
	return slices.Clone(cfg.Defaults.Workspace), nil
}

func describePackages(list []string) string {
	if len(list) == 0 {
		return "no packages"
	}
	return "the packages " + strings.Join(list, ", ")
}

// adminKey reports whether there is an administrative key to copy into a new
// account.
//
// A workspace is reachable because that key is authorised on it. With nothing
// to copy the account would be created with no way in, which is worse than
// refusing to declare it.
func adminKey(m config.Machine) error {
	if m.Key != "" {
		if _, err := keys.PublicFor(m.Key); err != nil {
			return fmt.Errorf(
				"machine %q names the key %s and it cannot be read: a workspace is reachable "+
					"because that key is copied into it. Run `devmachine setup` to give the machine a key: %w",
				m.Name, m.Key, err)
		}
		return nil
	}

	held, err := agentKeys()
	if err != nil {
		return fmt.Errorf("reading the SSH agent: %w", err)
	}
	if len(held) == 0 {
		return fmt.Errorf(
			"machine %q has no key, and the SSH agent holds none either: a workspace is "+
				"reachable because a key is copied into it, and there is nothing to copy. "+
				"Run `devmachine setup`", m.Name)
	}
	return nil
}

func newWorkspacesRmCmd(opts *options) *cobra.Command {
	var yes bool

	c := &cobra.Command{
		Use:   "rm <name>",
		Short: "Forget a workspace, leaving its account on the machine",
		Long: "Takes the workspace out of the configuration. The Linux account, " +
			"its home and its files stay on the machine.\n\n" +
			"Deleting somebody's home is not something a configuration edit " +
			"should do, and `sync` could not put it back.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			w, err := cfg.Workspace(args[0])
			if err != nil {
				return err
			}

			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(),
					"Forget the workspace "+w.Name+"? Its account and home stay on the machine.")
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}
			if err := config.RemoveWorkspace(dir, w.Name); err != nil {
				return err
			}

			cmd.Printf("%s is out of the configuration.\n", w.Name)
			cmd.Printf("The account %s, its home and its files are still on the machine %s.\n",
				w.LinuxUser(), machineNameOf(cfg, w))
			cmd.Println("Remove them there by hand if you really want them gone.")
			return nil
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "forget it without asking")
	return c
}
