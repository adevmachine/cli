package commands

import (
	"github.com/adevmachine/cli/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect the configuration this run would use",
	}
	cmd.AddCommand(newConfigPathCmd(opts), newConfigShowCmd(opts))
	return cmd
}

func newConfigPathCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the configuration directory and the rule that chose it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, source, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), struct {
					Path   string `json:"path"`
					Source string `json:"source"`
				}{dir, string(source)})
			}
			cmd.Printf("%s (from %s)\n", dir, source)
			return nil
		},
	}
}

func newConfigShowCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the effective configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			c, err := config.Load(dir)
			if err != nil {
				return err
			}
			// Validation runs here rather than in Load: reading a file and
			// judging its contents are different failures, and only this one
			// can tell the user what to change.
			if err := c.Validate(); err != nil {
				return err
			}

			addresses := make([]string, 0, len(c.Hosts))
			for _, h := range c.Hosts {
				addresses = append(addresses, h.Address)
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), struct {
					Hosts       []string `json:"hosts"`
					Domain      string   `json:"domain"`
					User        string   `json:"user"`
					Port        int      `json:"port"`
					DNSProvider string   `json:"dns_provider"`
				}{addresses, c.Domain, c.User, c.Port, c.DNSProvider})
			}

			for i, a := range addresses {
				cmd.Printf("host %d  %s\n", i+1, a)
			}
			cmd.Printf("domain  %s\n", c.Domain)
			cmd.Printf("user    %s\n", c.User)
			cmd.Printf("port    %d\n", c.Port)
			if c.DNSProvider != "" {
				cmd.Printf("dns     %s\n", c.DNSProvider)
			}
			return nil
		},
	}
}
