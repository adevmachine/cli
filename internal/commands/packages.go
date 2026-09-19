package commands

import (
	"fmt"
	"path/filepath"

	"github.com/adevmachine/cli/internal/packages"
	"github.com/spf13/cobra"
)

func newPackagesCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "packages",
		Short: "The recipes a machine and its workspaces are built from",
	}
	cmd.AddCommand(
		newPackagesNewCmd(opts),
		newPackagesValidateCmd(opts),
		newPackagesSchemaCmd(opts),
	)
	return cmd
}

func newPackagesNewCmd(opts *options) *cobra.Command {
	var scope, into string

	c := &cobra.Command{
		Use:   "new <name>",
		Short: "Write a new package that already validates",
		Long: "The skeleton installs something real rather than carrying a " +
			"comment telling you where to type. It is a starting point you " +
			"edit, and `packages validate` is what says when it is right.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			dir := filepath.Join(into, name)
			if err := packages.WriteSkeleton(dir, name, scope); err != nil {
				return err
			}
			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), struct {
					Path  string `json:"path"`
					Name  string `json:"name"`
					Scope string `json:"scope"`
				}{dir, name, scope})
			}
			cmd.Printf("wrote %s\n", dir)
			return nil
		},
	}
	c.Flags().StringVar(&scope, "scope", packages.ScopeMachine,
		fmt.Sprintf("where it is installed: %s or %s", packages.ScopeMachine, packages.ScopeWorkspace))
	c.Flags().StringVar(&into, "into", ".", "the directory to write the package into")
	return c
}

func newPackagesValidateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <dir>",
		Short: "Check a package, and say what is wrong and where",
		Long: "Every problem is reported at once, not the first: correcting " +
			"one at a time is four round trips for one answer's worth of work.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]
			problems, err := packages.Validate(dir)
			if err != nil {
				return err
			}

			if opts.format == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), struct {
					OK       bool               `json:"ok"`
					Package  string             `json:"package"`
					Problems []packages.Problem `json:"problems"`
				}{len(problems) == 0, dir, problems}); err != nil {
					return err
				}
			} else if len(problems) == 0 {
				cmd.Printf("%s is fine\n", dir)
			} else {
				for _, p := range problems {
					cmd.Printf("%s%c%s\n", dir, filepath.Separator, p.Error())
				}
			}

			if len(problems) > 0 {
				return fmt.Errorf("%s has %d problem(s)", dir, len(problems))
			}
			return nil
		},
	}
}

func newPackagesSchemaCmd(opts *options) *cobra.Command {
	var asJSON bool

	c := &cobra.Command{
		Use:   "schema",
		Short: "Print the package.yml format this CLI reads",
		Long: "Whoever writes a recipe asks the binary what the format is, " +
			"rather than a page that drifts the moment the format changes.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			schema := packages.Schema()
			if asJSON || opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), schema)
			}

			cmd.Printf("formats read: %v\n\n", schema.Formats)
			cmd.Printf("%-16s %-9s %s\n", "field", "required", "what it is")
			for _, f := range schema.Fields {
				required := "no"
				if f.Required {
					required = "yes"
				}
				cmd.Printf("%-16s %-9s %s\n", f.Name, required, f.Summary)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print the format as JSON")
	return c
}
