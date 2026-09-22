package commands

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/dns"
	"github.com/adevmachine/cli/internal/expose"
	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
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
	for _, f := range plan.OnMachine.Ordered {
		if f.Manifest.Name != "caddy" {
			continue
		}
		p := f.Manifest.Provides["sites.d"]
		if p == "" {
			return "", errors.New("the caddy package on this machine does not declare a sites.d extension point")
		}
		return p, nil
	}
	return "", fmt.Errorf(
		"caddy is not on %s: add it to the machine's packages and run `devmachine sync` first", machine.Name)
}

func newExposeAddCmd(opts *options) *cobra.Command {
	var host string
	var check, yes bool

	c := &cobra.Command{
		Use:   "add <workspace> <port>",
		Short: "Publish a workspace's port as a public hostname",
		Long: "Writes a Caddy block through the extension point the caddy " +
			"package declares, and reloads Caddy. Before writing anything it " +
			"asks — because whatever is behind the port becomes reachable by " +
			"anybody who learns the hostname.",
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

			tgt, err := workspaceTarget(opts, workspace)
			if err != nil {
				return err
			}
			site := expose.Site{Host: host, Port: port, Workspace: workspace}
			fileName := expose.FileName(site)
			if fileName == "" {
				return fmt.Errorf("%q is not a usable hostname", host)
			}

			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			sitesDir, err := caddySitesDir(cmd.Context(), dir, cfg, tgt.machine)
			if err != nil {
				return err
			}

			question := fmt.Sprintf(
				"Publish %s:%d as https://%s, reachable by anybody who learns that hostname?",
				workspace, port, host)
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

			client, _, err := dial(cmd.Context(), tgt.machine, "")
			if err != nil {
				return err
			}
			defer client.Close()

			if err := writeSite(cmd.Context(), client, sitesDir, fileName, expose.Render(site)); err != nil {
				return err
			}
			record(opts, tgt, fmt.Sprintf("expose add %s %d --host %s", workspace, port, host), true)

			if err := pointDNSAtMachine(cmd.Context(), dir, tgt, host, client, cmd.ErrOrStderr()); err != nil {
				return err
			}

			cmd.Printf("published https://%s -> 127.0.0.1:%d on %s\n", host, port, tgt.machine.Name)
			return nil
		},
	}
	c.Flags().StringVar(&host, "host", "", "the public hostname this port answers to (required)")
	c.Flags().BoolVar(&check, "check", false, "say what would happen, and change nothing")
	c.Flags().BoolVar(&yes, "yes", false, "publish without asking")
	return c
}

// writeSite writes the rendered block into sitesDir and reloads Caddy, so the
// site takes effect immediately instead of waiting for Caddy's own poll.
func writeSite(ctx context.Context, client remote.Client, sitesDir, fileName, block string) error {
	full := path.Join(sitesDir, fileName)
	script := fmt.Sprintf("mkdir -p %s && cat > %s && systemctl reload caddy", sitesDir, full)
	if _, err := client.RunInput(ctx, script, strings.NewReader(block)); err != nil {
		return fmt.Errorf("writing %s on the machine: %w", full, err)
	}
	return nil
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
}

var reHost = regexp.MustCompile(`(?m)^([^\s{#]+)\s*\{`)
var rePort = regexp.MustCompile(`reverse_proxy 127\.0\.0\.1:(\d+)`)
var reWorkspace = regexp.MustCompile(`(?m)^#\s*workspace:\s*(\S+)`)

// parseSites reads every *.caddy file `expose` wrote out of sites.d's
// concatenated content — one file per site, so a block's own braces mark
// where the next one starts.
func parseSites(blocks []string) []site {
	var out []site
	for _, block := range blocks {
		hm := reHost.FindStringSubmatch(block)
		pm := rePort.FindStringSubmatch(block)
		if hm == nil || pm == nil {
			continue
		}
		s := site{Host: hm[1]}
		s.Port, _ = strconv.Atoi(pm[1])
		if wm := reWorkspace.FindStringSubmatch(block); wm != nil {
			s.Workspace = wm[1]
		}
		out = append(out, s)
	}
	return out
}

// listSites reads sites.d and returns every site written there. A machine
// with no caddy, or with an empty sites.d, reports zero sites rather than an
// error: an empty list is a fine answer to "what is published".
func listSites(ctx context.Context, client remote.Client, sitesDir string) ([]site, error) {
	listing, err := client.Run(ctx, fmt.Sprintf("ls -1 %s 2>/dev/null", sitesDir))
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", sitesDir, err)
	}
	var blocks []string
	for _, name := range strings.Split(strings.TrimSpace(listing), "\n") {
		name = strings.TrimSpace(name)
		if name == "" || !strings.HasSuffix(name, ".caddy") {
			continue
		}
		content, err := client.Run(ctx, fmt.Sprintf("cat %s", path.Join(sitesDir, name)))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		blocks = append(blocks, content)
	}
	return parseSites(blocks), nil
}

func newExposeListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Every site currently published through Caddy",
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
			sitesDir, err := caddySitesDir(cmd.Context(), dir, cfg, tgt.machine)
			if err != nil {
				return err
			}
			client, _, err := dial(cmd.Context(), tgt.machine, "")
			if err != nil {
				return err
			}
			defer client.Close()

			sites, err := listSites(cmd.Context(), client, sitesDir)
			if err != nil {
				return err
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), sites)
			}
			if len(sites) == 0 {
				cmd.Println("Nothing is published.")
				return nil
			}
			for _, s := range sites {
				workspace := s.Workspace
				if workspace == "" {
					workspace = "-"
				}
				cmd.Printf("%-28s %-6d %s\n", s.Host, s.Port, workspace)
			}
			return nil
		},
	}
}

func newExposeRmCmd(opts *options) *cobra.Command {
	var check, yes bool

	c := &cobra.Command{
		Use:   "rm <host>",
		Short: "Stop publishing a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			fileName := expose.FileName(expose.Site{Host: host})
			if fileName == "" {
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
			sitesDir, err := caddySitesDir(cmd.Context(), dir, cfg, tgt.machine)
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

			client, _, err := dial(cmd.Context(), tgt.machine, "")
			if err != nil {
				return err
			}
			defer client.Close()

			full := path.Join(sitesDir, fileName)
			script := fmt.Sprintf("rm -f %s && systemctl reload caddy", full)
			if _, err := client.Run(cmd.Context(), script); err != nil {
				record(opts, tgt, "expose rm "+host, false)
				return fmt.Errorf("removing %s on the machine: %w", full, err)
			}
			record(opts, tgt, "expose rm "+host, true)

			cmd.Printf("%s is no longer published.\n", host)
			return nil
		},
	}
	c.Flags().BoolVar(&check, "check", false, "say what would happen, and change nothing")
	c.Flags().BoolVar(&yes, "yes", false, "remove without asking")
	return c
}
