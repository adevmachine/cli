package commands

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/dns"
	"github.com/adevmachine/cli/internal/expose"
	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
	"github.com/adevmachine/cli/internal/repo"
	"github.com/spf13/cobra"
)

func newExposeCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "expose",
		Short: "Publish a local port on the internet as an HTTPS hostname",
		Long: "HTTPS only: Caddy terminates TLS for HTTP. Proxying raw TCP or " +
			"UDP needs a plugin and a custom build — the same maintenance cost " +
			"this project already refused for wildcard certificates.\n\n" +
			"Whatever is behind the port becomes reachable by anybody who " +
			"learns the hostname. `devmachine tunnel` is the way to reach " +
			"something without putting it on the internet.",
	}
	cmd.AddCommand(newExposeAddCmd(opts), newExposeListCmd(opts), newExposeRmCmd(opts))
	return cmd
}

// caddySitesDir asks the machine's plan for the caddy package's `sites.d`
// extension point.
//
// `expose` never guesses the path: sites.d belongs to caddy, and caddy might
// not be on this machine at all.
func caddySitesDir(ctx context.Context, dir string, cfg config.Config, machine config.Machine) (string, error) {
	store, err := openStore(ctx, dir, cfg.Packages)
	if err != nil {
		return "", err
	}
	plan, err := packages.ResolveMachine(store, cfg, machine, version)
	if err != nil {
		return "", err
	}
	if plan.SitesDir == "" {
		return "", fmt.Errorf(
			"caddy is not on %s: add it with `devmachine packages add caddy --machine %s` and run `devmachine sync`",
			machine.Name, machine.Name)
	}
	return plan.SitesDir, nil
}

func newExposeAddCmd(opts *options) *cobra.Command {
	var host string
	var check, yes, publish bool

	c := &cobra.Command{
		Use:   "add <workspace> <port>",
		Short: "Record a workspace's port as a public hostname; sync publishes it",
		Long: "Records the route in the configuration. `devmachine sync` writes " +
			"the Caddy block through the extension point the caddy package " +
			"declares, and reloads Caddy. Before recording anything it asks — " +
			"because whatever is behind the port becomes reachable by anybody " +
			"who learns the hostname.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspace := args[0]
			port, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("%q is not a port number", args[1])
			}
			if host == "" {
				return errors.New("--host is required: the public hostname this port answers to")
			}
			if !config.UsableHost(host) {
				return fmt.Errorf("%q is not a usable hostname", host)
			}

			tgt, err := workspaceTarget(opts, workspace)
			if err != nil {
				return err
			}

			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			if _, err := caddySitesDir(cmd.Context(), dir, cfg, tgt.machine); err != nil {
				return err
			}

			question := fmt.Sprintf(
				"Publish %s:%d as https://%s, reachable by anybody who learns that hostname?",
				workspace, port, host)
			if check {
				cmd.Println("would " + question)
				return nil
			}
			if !publish {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(), question)
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}

			if err := config.AddRoute(dir, workspace, config.Route{Host: host, Port: port}); err != nil {
				return err
			}
			record(opts, tgt, fmt.Sprintf("expose add %s %d --host %s", workspace, port, host), true)
			repo.AutoCommit(cmd.Context(), dir, fmt.Sprintf("chore(config): expose %s", host))

			var client remote.Client
			if c, _, err := dial(cmd.Context(), tgt.machine, ""); err == nil {
				client = c
				defer client.Close()
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s could not be reached (%v); the DNS record is printed to create by hand\n", tgt.machine.Name, err)
			}
			if err := pointDNSAtMachine(cmd.Context(), dir, tgt, host, client, cmd.ErrOrStderr()); err != nil {
				return err
			}

			cmd.Printf("recorded https://%s -> %s:%d in the configuration. Run `devmachine sync` to publish it on %s.\n",
				host, workspace, port, tgt.machine.Name)
			return nil
		},
	}
	c.Flags().StringVar(&host, "host", "", "the public hostname this port answers to (required)")
	c.Flags().BoolVar(&check, "check", false, "say what would happen, and change nothing")
	c.Flags().BoolVar(&yes, "yes", false, "skip local-write questions; never publish")
	c.Flags().BoolVar(&publish, "publish", false, "publish the site without asking")
	return c
}

// pointDNSAtMachine reuses the same dns.Choose and Upsert path `dns add`
// uses: an installed provider gets the record written, and with none
// installed, the exact record to create by hand is printed instead of
// refusing. A name that does not resolve yet fails minutes later, in Caddy's
// certificate log, where nobody is looking.
func pointDNSAtMachine(ctx context.Context, dir string, tgt target, host string, client remote.Client, out interface {
	Write([]byte) (int, error)
}) error {
	choice, err := dns.Choose(ctx, dir, tgt.machine.Name, host, "", client, out)
	if err != nil {
		return err
	}
	address, err := firstAddress(tgt.machine)
	if err != nil {
		return err
	}
	rec := dns.Record{Name: labelFor(host, choice.Zone), Type: "A", Value: address}
	return choice.Provider.Upsert(ctx, choice.Zone, rec)
}

// site is one row `expose list` reports.
type site struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Workspace string `json:"workspace,omitempty"`
	// Status is one of: published, pending, unmanaged, differs, unknown.
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// listSites reads sites.d and returns every site written there. A machine
// with no caddy, or with an empty sites.d, reports zero sites rather than an
// error: an empty list is a fine answer to "what is published".
func listSites(ctx context.Context, client remote.Client, sitesDir string) ([]expose.Site, error) {
	listing, err := client.Run(ctx, fmt.Sprintf("ls -1 %s 2>/dev/null", sitesDir))
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", sitesDir, err)
	}
	var sites []expose.Site
	for _, name := range strings.Split(strings.TrimSpace(listing), "\n") {
		name = strings.TrimSpace(name)
		if name == "" || !strings.HasSuffix(name, ".caddy") {
			continue
		}
		content, err := client.Run(ctx, fmt.Sprintf("cat %s", path.Join(sitesDir, name)))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		sites = append(sites, expose.Parse(content)...)
	}
	return sites, nil
}

// reconcile lines the configuration up against the machine, by host.
//
// Reachable and empty are different answers: the machine side is nil when it
// could not be asked, and then nothing is "pending", it is "unknown".
func reconcile(cfg config.Config, machine string, onMachine []expose.Site, reachable bool) []site {
	seen := map[string]bool{}
	var rows []site
	for _, w := range cfg.WorkspacesOn(machine) {
		for _, r := range w.Routes {
			seen[r.Host] = true
			row := site{Host: r.Host, Port: r.Port, Workspace: w.Name}
			switch {
			case !reachable:
				row.Status = "unknown"
				row.Note = "the machine could not be asked"
			default:
				found, ok := find(onMachine, r.Host)
				switch {
				case !ok:
					row.Status = "pending"
					row.Note = "not on the machine yet: run `devmachine sync`"
				case found.Port != r.Port || found.Workspace != w.Name:
					row.Status = "differs"
					row.Note = fmt.Sprintf("the machine has port %d for %s: run `devmachine sync`", found.Port, orDash(found.Workspace))
				default:
					row.Status = "published"
				}
			}
			rows = append(rows, row)
		}
	}
	for _, s := range onMachine {
		if seen[s.Host] {
			continue
		}
		rows = append(rows, site{Host: s.Host, Port: s.Port, Workspace: s.Workspace, Status: "unmanaged",
			Note: fmt.Sprintf("only on the machine, and gone on a rebuild: adopt it with `devmachine expose add %s %d --host %s`",
				orDash(s.Workspace), s.Port, s.Host)})
	}
	return rows
}

func find(sites []expose.Site, host string) (expose.Site, bool) {
	for _, s := range sites {
		if s.Host == host {
			return s, true
		}
	}
	return expose.Site{}, false
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newExposeListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Every site the configuration and the machine agree, or disagree, about",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tgt, err := machineTarget(opts)
			if err != nil {
				return err
			}
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			var rows []site
			sitesDir, err := caddySitesDir(cmd.Context(), dir, cfg, tgt.machine)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s cannot be asked: %v\n", tgt.machine.Name, err)
				rows = reconcile(cfg, tgt.machine.Name, nil, false)
			} else if client, _, err := dial(cmd.Context(), tgt.machine, ""); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s could not be reached (%v)\n", tgt.machine.Name, err)
				rows = reconcile(cfg, tgt.machine.Name, nil, false)
			} else {
				defer client.Close()
				sites, err := listSites(cmd.Context(), client, sitesDir)
				if err != nil {
					return err
				}
				rows = reconcile(cfg, tgt.machine.Name, sites, true)
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), rows)
			}
			if len(rows) == 0 {
				cmd.Println("Nothing is published.")
				return nil
			}
			for _, r := range rows {
				cmd.Printf("%-28s %-6d %-10s %-10s %s\n", r.Host, r.Port, orDash(r.Workspace), r.Status, r.Note)
			}
			return nil
		},
	}
}

func newExposeRmCmd(opts *options) *cobra.Command {
	var check, yes bool

	c := &cobra.Command{
		Use:   "rm <host>",
		Short: "Stop publishing a site at the next sync",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			if !config.UsableHost(host) {
				return fmt.Errorf("%q is not a usable hostname", host)
			}

			tgt, err := machineTarget(opts)
			if err != nil {
				return err
			}
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			question := fmt.Sprintf("Stop publishing https://%s ?", host)
			if check {
				cmd.Println("would " + question)
				return nil
			}
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(), question)
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}

			owner, err := config.RemoveRoute(dir, host)
			if err != nil {
				where := "<caddy's sites.d>/" + expose.FileName(expose.Site{Host: host})
				if sitesDir, derr := caddySitesDir(cmd.Context(), dir, cfg, tgt.machine); derr == nil {
					where = path.Join(sitesDir, expose.FileName(expose.Site{Host: host}))
				}
				return fmt.Errorf("%w; a site the old `expose` wrote is a file on the machine, not a line here: "+
					"adopt it with `devmachine expose add <workspace> <port> --host %s`, or remove it there with "+
					"`devmachine run 'rm %s && systemctl reload caddy'`",
					err, host, where)
			}
			record(opts, target{machine: tgt.machine, workspace: owner}, "expose rm "+host, true)
			repo.AutoCommit(cmd.Context(), dir, fmt.Sprintf("chore(config): stop exposing %s", host))

			cmd.Printf("%s is no longer in the configuration (it was %s's). Run `devmachine sync` to take it off %s.\n",
				host, owner, tgt.machine.Name)
			return nil
		},
	}
	c.Flags().BoolVar(&check, "check", false, "say what would happen, and change nothing")
	c.Flags().BoolVar(&yes, "yes", false, "remove without asking")
	return c
}
