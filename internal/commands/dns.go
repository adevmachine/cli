package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/dns"
	"github.com/adevmachine/cli/internal/remote"
	"github.com/spf13/cobra"
)

func newDNSCmd(opts *options) *cobra.Command {
	var providerFlag, zoneFlag string

	cmd := &cobra.Command{
		Use:   "dns",
		Short: "DNS for the names this setup serves",
	}
	cmd.PersistentFlags().StringVar(&providerFlag, "dns-provider", "",
		"act through this provider instead of asking which installed one holds the zone")
	cmd.PersistentFlags().StringVar(&zoneFlag, "zone", "",
		"the zone to act in, for a provider whose token cannot list it")

	cmd.AddCommand(newDNSStatusCmd(opts))
	cmd.AddCommand(newDNSProvidersCmd(opts))
	cmd.AddCommand(newDNSListCmd(opts, &providerFlag, &zoneFlag))
	cmd.AddCommand(newDNSCheckCmd(opts, &providerFlag, &zoneFlag))
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

// chooseDNS resolves a name to the provider that would act on it, dialling
// the machine along the way. It is a seam: a test replaces it to drive
// dns list/check/add/rm without a machine to reach.
var chooseDNS = defaultChooseDNS

func defaultChooseDNS(ctx context.Context, opts *options, name, providerFlag, zoneFlag string, out io.Writer,
) (dns.Choice, target, error) {
	tgt, err := machineTarget(opts)
	if err != nil {
		return dns.Choice{}, target{}, err
	}
	dir, _, err := config.Dir(opts.configDir)
	if err != nil {
		return dns.Choice{}, target{}, err
	}

	client, _, err := dial(ctx, tgt.machine, "")
	if err != nil {
		// A DNS command with the machine down still has manual to fall back
		// to: dns.Choose treats a nil client as "nothing to ask", not as a
		// reason to stop.
		client = nil
	}

	choice, err := dns.Choose(ctx, dir, tgt.machine.Name, name, providerFlag, client, out)
	if err != nil {
		return dns.Choice{}, target{}, err
	}
	if zoneFlag != "" {
		choice.Zone = zoneFlag
	}
	return choice, tgt, nil
}

// announce prints which provider answered and why, always on stderr: stdout
// is data, and which provider was asked is diagnostics.
func announce(w io.Writer, choice dns.Choice) {
	fmt.Fprintf(w, "%s: %s\n", choice.Name, choice.Why)
}

// labelFor turns a full name into the label a provider's model expects: the
// zone itself becomes "@", and anything else has the zone stripped off.
func labelFor(name, zone string) string {
	if name == zone {
		return "@"
	}
	return strings.TrimSuffix(name, "."+zone)
}

func newDNSListCmd(opts *options, providerFlag, zoneFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list [zone]",
		Short: "Every record a provider holds for a zone",
		Long: "This is a different question from `dns status`: it asks the " +
			"registrar, not the public internet, so it sees a record that has " +
			"not propagated yet and says nothing about one that has.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			zone := ""
			if len(args) == 1 {
				zone = args[0]
			}
			if zone == "" {
				cfg, err := loadConfig(opts)
				if err != nil {
					return err
				}
				if cfg.Domain == "" {
					return errors.New("no zone given and no `domain` in the configuration")
				}
				zone = cfg.Domain
			}

			choice, _, err := chooseDNS(cmd.Context(), opts, zone, *providerFlag, *zoneFlag, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			announce(cmd.ErrOrStderr(), choice)

			records, err := choice.Provider.List(cmd.Context(), choice.Zone)
			if err != nil {
				return err
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), struct {
					Zone     string       `json:"zone"`
					Provider string       `json:"provider"`
					Records  []dns.Record `json:"records"`
				}{choice.Zone, choice.Name, records})
			}
			for _, r := range records {
				cmd.Printf("%-16s %-6s %-24s %d\n", r.Name, r.Type, r.Value, r.TTL)
			}
			return nil
		},
	}
}

func newDNSCheckCmd(opts *options, providerFlag, zoneFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "check <name>",
		Short: "Whether a name is pointed at the registrar",
		Long: "`dns status` asks the public internet, which can still say no " +
			"while the record is sitting right there, not yet propagated. This " +
			"asks the registrar directly.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			choice, _, err := chooseDNS(cmd.Context(), opts, name, *providerFlag, *zoneFlag, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			announce(cmd.ErrOrStderr(), choice)

			records, err := choice.Provider.List(cmd.Context(), choice.Zone)
			if err != nil {
				return err
			}

			label := labelFor(name, choice.Zone)
			var matched []dns.Record
			for _, r := range records {
				if r.Name == label {
					matched = append(matched, r)
				}
			}
			found := len(matched) > 0

			if opts.format == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), struct {
					Name     string       `json:"name"`
					Zone     string       `json:"zone"`
					Provider string       `json:"provider"`
					Found    bool         `json:"found"`
					Records  []dns.Record `json:"records"`
				}{name, choice.Zone, choice.Name, found, matched}); err != nil {
					return err
				}
			} else if found {
				cmd.Printf("%s is pointed, through %s:\n", name, choice.Name)
				for _, r := range matched {
					cmd.Printf("  %-6s %-24s %d\n", r.Type, r.Value, r.TTL)
				}
			} else {
				cmd.Printf("%s is not pointed, through %s\n", name, choice.Name)
			}

			if !found {
				return fmt.Errorf("%s is not pointed at %s", name, choice.Name)
			}
			return nil
		},
	}
}
