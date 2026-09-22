package commands

import (
	"context"
	"errors"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/dns"
	"github.com/adevmachine/cli/internal/remote"
	"github.com/spf13/cobra"
)

func newDNSCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dns",
		Short: "DNS for the names this setup serves",
	}
	cmd.AddCommand(newDNSStatusCmd(opts))
	cmd.AddCommand(newDNSProvidersCmd(opts))
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

// dnsZoner is what `dns providers` needs from an installed provider: enough
// to say what it holds, without depending on dns.External directly.
type dnsZoner interface {
	Name() string
	Zones(ctx context.Context) ([]string, error)
}

// dnsInstalled is the seam a test replaces so `dns providers` never dials a
// machine for real. It wraps dns.Installed behind an interface so a test can
// hand it a stub provider instead of a real dns.External.
var dnsInstalled = func(dir, machine string, client remote.Client) ([]dnsZoner, error) {
	providers, err := dns.Installed(dir, machine, client)
	if err != nil {
		return nil, err
	}
	out := make([]dnsZoner, len(providers))
	for i, p := range providers {
		out[i] = p
	}
	return out, nil
}

// providerRow is one installed provider, as `dns providers` reports it.
type providerRow struct {
	Name  string   `json:"name"`
	Zones []string `json:"zones"`
	Error string   `json:"error,omitempty"`
}

func newDNSProvidersCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "Every installed DNS provider, and the zones it can see",
		Long: "The one read that shows the whole picture, and the first thing " +
			"to run when a DNS command did not do what was expected.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tgt, err := machineTarget(opts)
			if err != nil {
				return err
			}
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			client, _, err := dial(cmd.Context(), tgt.machine, "")
			if err != nil {
				return err
			}
			defer client.Close()

			providers, err := dnsInstalled(dir, tgt.machine.Name, client)
			if err != nil {
				return err
			}

			rows := make([]providerRow, 0, len(providers))
			for _, p := range providers {
				row := providerRow{Name: p.Name()}
				zones, zerr := p.Zones(cmd.Context())
				if zerr != nil {
					row.Error = zerr.Error()
				} else {
					row.Zones = zones
				}
				rows = append(rows, row)
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), struct {
					Providers []providerRow `json:"providers"`
				}{rows})
			}

			if len(rows) == 0 {
				cmd.Println("No DNS provider is installed. Run `devmachine packages list` to see what is available.")
				return nil
			}
			for _, row := range rows {
				if row.Error != "" {
					cmd.Printf("%-14s %s\n", row.Name, row.Error)
					continue
				}
				cmd.Printf("%-14s %s\n", row.Name, strings.Join(row.Zones, ", "))
			}
			return nil
		},
	}
}
