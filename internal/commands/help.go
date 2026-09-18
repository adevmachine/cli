package commands

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// commandJSON is one command in the machine-readable surface.
type commandJSON struct {
	Name    string        `json:"name"`
	Path    string        `json:"path"`
	Summary string        `json:"summary"`
	Args    string        `json:"args,omitempty"`
	Flags   []flagJSON    `json:"flags,omitempty"`
	Sub     []commandJSON `json:"commands,omitempty"`
}

type flagJSON struct {
	Name    string `json:"name"`
	Usage   string `json:"usage"`
	Default string `json:"default,omitempty"`
}

type surfaceJSON struct {
	Name     string        `json:"name"`
	Version  string        `json:"version"`
	Flags    []flagJSON    `json:"flags"`
	Commands []commandJSON `json:"commands"`
}

func newHelpJSONCmd(opts *options) *cobra.Command {
	var asJSON bool

	c := &cobra.Command{
		Use:   "help [command]",
		Short: "Print the command surface",
		Long: "With --json it prints the whole tree in one document, which is " +
			"what lets a script or an agent discover what exists without " +
			"parsing help text.",
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			if asJSON || opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), surface(root))
			}
			if len(args) > 0 {
				target, _, err := root.Find(args)
				if err != nil {
					return err
				}
				return target.Help()
			}
			return root.Help()
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print the surface as JSON")
	return c
}

// surface walks the command tree.
func surface(root *cobra.Command) surfaceJSON {
	return surfaceJSON{
		Name:     root.Name(),
		Version:  version,
		Flags:    flagsOf(root),
		Commands: childrenOf(root),
	}
}

func childrenOf(parent *cobra.Command) []commandJSON {
	var out []commandJSON
	for _, c := range parent.Commands() {
		// The generated completion command is cobra's, not ours, and listing
		// it would suggest it is part of what this CLI offers.
		if c.Hidden || c.Name() == "completion" {
			continue
		}
		out = append(out, commandJSON{
			Name:    c.Name(),
			Path:    c.CommandPath(),
			Summary: c.Short,
			Args:    strings.TrimSpace(strings.TrimPrefix(c.Use, c.Name())),
			Flags:   flagsOf(c),
			Sub:     childrenOf(c),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func flagsOf(c *cobra.Command) []flagJSON {
	var out []flagJSON
	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		out = append(out, flagJSON{Name: f.Name, Usage: f.Usage, Default: f.DefValue})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// WriteSurface prints one command path per line, deepest last. It is what
// SURFACE.txt holds, so a diff shows when the command surface changes.
func WriteSurface(w io.Writer) error {
	var walk func(c *cobra.Command) error
	walk = func(c *cobra.Command) error {
		if _, err := fmt.Fprintln(w, c.CommandPath()); err != nil {
			return err
		}
		children := c.Commands()
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, sub := range children {
			if sub.Hidden || sub.Name() == "completion" {
				continue
			}
			if err := walk(sub); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(NewRootCmd())
}
