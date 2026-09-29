package commands

import (
	"fmt"
	"io"

	"github.com/mydevmachine/devmachine/internal/aliases"
	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/spf13/cobra"
)

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
	path, err := aliases.DefaultPath()
	if err != nil {
		return err
	}
	block, err := aliases.Render(cfg)
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
			"and `mosh alice-devmachine` work from an ordinary terminal.\n\n" +
			"--write puts them in your SSH configuration, between two markers. " +
			"Everything outside those markers is left exactly as it was: that " +
			"file holds hosts this CLI knows nothing about.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAliases(cmd, opts, write, path, check, yes)
		},
	}
	c.Flags().BoolVar(&write, "write", false, "write the block into an SSH configuration")
	c.Flags().StringVar(&path, "path", "", "the file to write (default: ~/.ssh/config)")
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
	found, err := aliases.List(cfg)
	if err != nil {
		return err
	}
	block, err := aliases.Render(cfg)
	if err != nil {
		return err
	}

	defaultPath, err := aliases.DefaultPath()
	if err != nil {
		return err
	}
	if write && path == "" {
		path = defaultPath
	}

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

		// Writing this by hand into the default file is exactly the answer
		// `setup` asks for. Recording it is what makes every later change
		// keep it up to date without asking again.
		if path == defaultPath && !cfg.SSHAliases {
			if err := config.SetSSHAliases(dir, true); err != nil {
				return err
			}
			cfg.SSHAliases = true
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
			cmd.Printf("%-24s %s@%s:%d\n", a.Name, a.User, a.Host, a.Port)
		}
		cmd.Printf("\nWritten into %s, between the devmachine markers.\n", path)
		if path == defaultPath && cfg.SSHAliases {
			cmd.Println("This will stay up to date automatically from now on.")
		}
	default:
		cmd.Printf("%s\n%s%s\n", aliases.Begin, block, aliases.End)
	}
	return nil
}
