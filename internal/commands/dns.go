package commands

import (
	"errors"

	"github.com/adevmachine/cli/internal/dns"
	"github.com/spf13/cobra"
)

func newDNSCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dns",
		Short: "DNS for the names this setup serves",
	}
	cmd.AddCommand(newDNSStatusCmd(opts))
	return cmd
}

func newDNSStatusCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status [host]",
		Short: "Check a name from outside: DNS, TLS and one request",
		Long: "With no host it checks the configured domain. The check is made " +
			"from this computer, not from the machine, so it sees what anybody " +
			"else would see.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := ""
			if len(args) == 1 {
				host = args[0]
			}
			if host == "" {
				cfg, err := loadConfig(opts)
				if err != nil {
					return err
				}
				if cfg.Domain == "" {
					return errors.New("no host given and no `domain` in the configuration")
				}
				host = cfg.Domain
			}

			report := dns.Status(cmd.Context(), host)

			if opts.format == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), struct {
					dns.Report
					OK bool `json:"ok"`
				}{report, report.OK()}); err != nil {
					return err
				}
			} else {
				cmd.Printf("%s\n", report.Host)
				for _, s := range report.Steps {
					cmd.Printf("  %-4s  %-5s  %s\n", s.Status, s.Name, s.Detail)
				}
			}

			if !report.OK() {
				return errors.New("the name is not serving")
			}
			return nil
		},
	}
}
