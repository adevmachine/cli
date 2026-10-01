package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mydevmachine/devmachine/internal/aliases"
	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/repo"
	"github.com/spf13/cobra"
)

// aliasCLI is the devmachine the aliases name as their ProxyCommand. It is a
// seam, so what a test writes does not depend on the PATH it runs with.
var aliasCLI = aliases.CLIPath

// refreshAliases keeps ~/.ssh/config's managed block in step with the
// configuration, for a person who already said yes once.
//
// It never asks: the answer is `cfg.SSHAliases`, recorded during setup or
// `machines add`. A configuration that never turned it on is left exactly as
// it always was — the pre-existing behaviour of every command that did not
// know this field existed.
func refreshAliases(cfg config.Config, out io.Writer) error {
	if !cfg.SSHAliases {
		return nil
	}
	path, err := aliases.PathFor(cfg)
	if err != nil {
		return err
	}
	block, err := aliases.Render(cfg, aliases.Options{CLI: aliasCLI()})
	if err != nil {
		return err
	}
	changed, err := aliases.WriteChanged(path, block)
	if err != nil {
		return err
	}
	if changed {
		fmt.Fprintf(out, "updated the SSH aliases in %s\n", path)
	}
	return nil
}

// reachLines is how to reach a workspace, printed at the end of whatever
// command just made it reachable for the first time (or changed the set of
// workspaces a machine answers for).
func reachLines(name string, sshAliases bool) []string {
	lines := []string{
		fmt.Sprintf("devmachine ssh %s (or `devmachine mosh %s`) reaches it from here", name, name),
	}
	if sshAliases {
		lines = append(lines, fmt.Sprintf(
			"ssh %s-devmachine reaches it from any terminal or editor (VS Code Remote-SSH, Zed, the macOS app)", name))
	}
	return lines
}

func newAliasesCmd(opts *options) *cobra.Command {
	var (
		write bool
		path  string
		check bool
		yes   bool
	)

	c := &cobra.Command{
		Use:   "aliases",
		Short: "The SSH host entries each workspace is reached by",
		Long: "Prints one Host entry per workspace, so `ssh alice-devmachine` " +
			"works from an ordinary terminal, an editor or an app.\n\n" +
			"When devmachine is on your PATH, each entry connects through " +
			"`devmachine ssh-proxy`, which picks the machine's address the moment " +
			"ssh connects: a private network going up or down never leaves a " +
			"stale address behind. Without it on PATH, the entry holds the " +
			"address that works now, and needs writing again when that changes.\n\n" +
			"--write puts them in your SSH configuration, between two markers. " +
			"Everything outside those markers is left exactly as it was: that " +
			"file holds hosts this CLI knows nothing about.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAliases(cmd, opts, write, path, check, yes)
		},
	}
	c.Flags().BoolVar(&write, "write", false, "write the block into an SSH configuration")
	c.Flags().StringVar(&path, "path", "",
		"the file the aliases live in from now on (default: ssh_aliases_path, else ~/.ssh/config)")
	c.Flags().BoolVar(&check, "check", false, "say what would be written, and write nothing")
	c.Flags().BoolVar(&yes, "yes", false, "write without asking")
	return c
}

func runAliases(cmd *cobra.Command, opts *options, write bool, path string, check, yes bool) error {
	if path != "" && !write {
		return fmt.Errorf("--path says where to write and nothing is being written: pass --write too")
	}

	dir, _, err := config.Dir(opts.configDir)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}
	form := aliases.Options{CLI: aliasCLI()}
	found, err := aliases.List(cfg, form)
	if err != nil {
		return err
	}
	block, err := aliases.Render(cfg, form)
	if err != nil {
		return err
	}

	current, err := aliases.PathFor(cfg)
	if err != nil {
		return err
	}
	if path == "" {
		path = current
	}
	if path, err = aliases.ExpandHome(path); err != nil {
		return err
	}
	moving := path != current

	written := false
	if write && !check {
		if !yes {
			// It is the person's own file, and one they may have written by
			// hand over years. Asking is the least this can do.
			ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(),
				fmt.Sprintf("Write %d alias(es) into %s? Everything outside the devmachine block is left alone.",
					len(found), path))
			if err != nil {
				return err
			}
			if !ok {
				return errDeclined
			}
		}
		if err := aliases.Write(path, block); err != nil {
			return err
		}
		written = true

		// Writing them by hand is exactly the answer `setup` asks for, and
		// the file they were written to is where they live from now on.
		// Recording both is what keeps every later change in that one file,
		// instead of a refresh writing a second copy into ~/.ssh/config.
		recorded := false
		if moving {
			if err := moveAliases(dir, &cfg, current, path); err != nil {
				return err
			}
			recorded = true
		}
		if !cfg.SSHAliases {
			if err := config.SetSSHAliases(dir, true); err != nil {
				return err
			}
			cfg.SSHAliases = true
			recorded = true
		}
		if recorded {
			repo.AutoCommit(cmd.Context(), dir, "chore(config): keep the SSH aliases in "+path)
		}
	}

	if opts.format == formatJSON {
		return writeJSON(cmd.OutOrStdout(), struct {
			Aliases    []aliases.Alias `json:"aliases"`
			Path       string          `json:"path,omitempty"`
			Written    bool            `json:"written"`
			SSHAliases bool            `json:"ssh_aliases"`
		}{found, path, written, cfg.SSHAliases})
	}

	switch {
	case len(found) == 0:
		cmd.Println("No workspace is configured, so there is no alias to write.")
		cmd.Println("`devmachine workspaces new <name>` makes one.")
	case check:
		cmd.Printf("would write %d alias(es) into %s:\n\n", len(found), path)
		cmd.Printf("%s\n%s%s\n", aliases.Begin, block, aliases.End)
		cmd.Println("\nNothing was written.")
	case written:
		for _, a := range found {
			if a.ProxyCommand != "" {
				cmd.Printf("%-24s %s@%s:%d, at the address that answers when ssh connects\n", a.Name, a.User, a.Host, a.Port)
				continue
			}
			cmd.Printf("%-24s %s@%s:%d\n", a.Name, a.User, a.Host, a.Port)
		}
		cmd.Printf("\nWritten into %s, between the devmachine markers.\n", path)
		if cfg.SSHAliases {
			cmd.Println("This will stay up to date automatically from now on.")
		}
	default:
		cmd.Printf("%s\n%s%s\n", aliases.Begin, block, aliases.End)
	}
	return nil
}

// moveAliases records the new file as the aliases' one home and empties the
// block in the file they lived in before, so ssh never reads two copies — the
// older one first, through an Include, would win.
func moveAliases(dir string, cfg *config.Config, from, to string) error {
	defaultPath, err := aliases.DefaultPath()
	if err != nil {
		return err
	}
	recorded := to
	if to == defaultPath {
		recorded = ""
	}
	if err := config.SetSSHAliasesPath(dir, recorded); err != nil {
		return err
	}
	cfg.SSHAliasesPath = recorded

	// Only a file that holds the block is touched: emptying one that never
	// had it would create it, or add markers to somebody's own file.
	body, err := os.ReadFile(from)
	if errors.Is(err, os.ErrNotExist) || (err == nil && !strings.Contains(string(body), aliases.Begin)) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", from, err)
	}
	if _, err := aliases.WriteChanged(from, ""); err != nil {
		return fmt.Errorf("emptying the aliases left in %s: %w", from, err)
	}
	return nil
}
