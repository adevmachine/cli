package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/dns"
	"github.com/mydevmachine/devmachine/internal/expose"
	"github.com/mydevmachine/devmachine/internal/packages"
	"github.com/mydevmachine/devmachine/internal/provision"
	"github.com/mydevmachine/devmachine/internal/remote"
	"github.com/mydevmachine/devmachine/internal/repo"
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

// machinePlan resolves what the machine is asked to run, the way sync does.
func machinePlan(ctx context.Context, dir string, cfg config.Config, machine config.Machine) (packages.MachinePlan, error) {
	store, err := openStore(ctx, dir, cfg.Packages)
	if err != nil {
		return packages.MachinePlan{}, err
	}
	return packages.ResolveMachine(store, cfg, machine, version)
}

// upstreamOf is the address a machine serving a site for m proxies to.
var upstreamOf = func(ctx context.Context, m config.Machine) (string, error) {
	return remote.Upstream(remote.ResolveAll(ctx, m))
}

// withUpstreams fills in the address of every machine the routes of one
// workspace come from, or of every workspace when it is "", and says, per
// machine, why one could not be found. A route left without one is left
// alone on the machine by both sync and `expose`.
func withUpstreams(ctx context.Context, cfg config.Config, plan packages.MachinePlan, workspace string) (packages.MachinePlan, map[string]error) {
	addresses := map[string]string{}
	failures := map[string]error{}
	plan.Routes = slices.Clone(plan.Routes)
	for i, r := range plan.Routes {
		if r.From == "" || (workspace != "" && r.Workspace != workspace) {
			continue
		}
		_, found := addresses[r.From]
		if _, failed := failures[r.From]; !found && !failed {
			m, err := cfg.Machine(r.From)
			if err == nil {
				addresses[r.From], err = upstreamOf(ctx, m)
			}
			if err != nil {
				failures[r.From] = err
			}
		}
		plan.Routes[i].Upstream = addresses[r.From]
	}
	return plan, failures
}

// servingMachines names the machines with caddy, other than this computer,
// in configuration order: where a site can be published.
func servingMachines(ctx context.Context, dir string, cfg config.Config) []string {
	var out []string
	for _, m := range cfg.Machines {
		if m.Self {
			continue
		}
		if plan, err := machinePlan(ctx, dir, cfg, m); err == nil && plan.SitesDir != "" {
			out = append(out, m.Name)
		}
	}
	return out
}

// caddySitesDir asks the machine's plan for the caddy package's `sites.d`
// extension point.
//
// `expose` never guesses the path: sites.d belongs to caddy, and caddy might
// not be on this machine at all.
func caddySitesDir(ctx context.Context, dir string, cfg config.Config, machine config.Machine) (string, error) {
	plan, err := machinePlan(ctx, dir, cfg, machine)
	if err != nil {
		return "", err
	}
	if plan.SitesDir == "" {
		return "", errNoCaddy{machine.Name}
	}
	return plan.SitesDir, nil
}

// errNoCaddy says the machine has no caddy in its package list, as opposed
// to its plan failing to resolve at all.
type errNoCaddy struct{ machine string }

func (e errNoCaddy) Error() string {
	return fmt.Sprintf("caddy is not on %s: add it with `devmachine packages add caddy --machine %s` and run `devmachine sync`",
		e.machine, e.machine)
}

// exposeResult is what `expose add` and `expose rm` print with --format json.
type exposeResult struct {
	Host      string `json:"host"`
	Port      int    `json:"port,omitempty"`
	Workspace string `json:"workspace"`
	// Via is the machine whose Caddy serves the site, when it is not the
	// workspace's own.
	Via string `json:"via,omitempty"`
	// Applied says Caddy on the machine already serves the change.
	Applied bool `json:"applied"`
	// Status is `published` or `removed` once applied, and `pending` while
	// only the configuration has the change.
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// routesChange is the routes file the workspace's share of cfg makes, the
// same one sync would write.
func routesChange(ctx context.Context, dir string, cfg config.Config, machine config.Machine, workspace string) (expose.FileChange, error) {
	plan, err := machinePlan(ctx, dir, cfg, machine)
	if err != nil {
		return expose.FileChange{}, err
	}
	plan, failures := withUpstreams(ctx, cfg, plan, workspace)
	change, err := provision.WorkspaceRoutes(plan, workspace)
	if err != nil {
		for _, r := range provision.UnresolvedRoutes(plan) {
			if r.Workspace == workspace && failures[r.From] != nil {
				return expose.FileChange{}, failures[r.From]
			}
		}
	}
	return change, err
}

// probeCommand asks a machine whether it reaches address:port at all. It
// answers `unknown` where it cannot tell, rather than claiming it does not.
func probeCommand(address string, port int) string {
	return fmt.Sprintf("if command -v bash >/dev/null 2>&1 && command -v timeout >/dev/null 2>&1; then "+
		"if timeout 3 bash -c '</dev/tcp/%s/%d' 2>/dev/null; then echo reachable; else echo unreachable; fi; "+
		"else echo unknown; fi", address, port)
}

// unreachableNote says what to check when the serving machine cannot reach
// the port it proxies to, or "" when it can or cannot tell. Caddy would
// otherwise answer every visitor with a bare 502.
func unreachableNote(ctx context.Context, client remote.Client, serving, from config.Machine, port int) string {
	address, err := upstreamOf(ctx, from)
	if err != nil {
		return ""
	}
	out, err := client.Run(ctx, probeCommand(address, port))
	if err != nil || strings.TrimSpace(out) != "unreachable" {
		return ""
	}
	return fmt.Sprintf("%s cannot reach %s at %s: check that the service listens on that address and not only on "+
		"127.0.0.1, that %s's firewall lets %s in, and that both are on the same private network",
		serving.Name, from.Name, net.JoinHostPort(address, strconv.Itoa(port)), from.Name, serving.Name)
}

// errNotApplied says the configuration has the change and the machine does
// not: the command still succeeded at recording it.
type errNotApplied struct{ reason string }

func (e errNotApplied) Error() string { return e.reason }

// applyRoutes puts the workspace's routes file on the machine and reloads
// Caddy, without running the machine's play: the file is rendered by the same
// code sync uses, so a later sync finds nothing to change.
//
// An errNotApplied means nothing on the machine changed and a sync will do
// it; any other error means Caddy was asked and refused.
func applyRoutes(ctx context.Context, client remote.Client, dir string, machine config.Machine, workspace string) error {
	if client == nil {
		return errNotApplied{fmt.Sprintf("%s could not be reached", machine.Name)}
	}
	if machine.Self {
		return errNotApplied{fmt.Sprintf("%s is this computer, where only sync writes Caddy's files", machine.Name)}
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	change, err := routesChange(ctx, dir, cfg, machine, workspace)
	if err != nil {
		return errNotApplied{err.Error()}
	}
	out, err := client.RunInput(ctx, expose.ApplyCommand, strings.NewReader(expose.ApplyScript(change, expose.Caddyfile)))
	if err != nil {
		return fmt.Errorf("writing %s on %s: %w %s", change.Path, machine.Name, err, strings.TrimSpace(out))
	}
	outcome, detail, err := expose.ParseApply(out)
	if err != nil {
		return err
	}
	switch outcome {
	case expose.Applied, expose.Unchanged:
		return nil
	case expose.NoCaddy:
		return errNotApplied{fmt.Sprintf("Caddy is not installed on %s yet", machine.Name)}
	case expose.Invalid:
		return fmt.Errorf("caddy refused the new %s, so the old one stays:\n%s", change.Path, detail)
	default:
		return fmt.Errorf("caddy did not reload, so the old %s is back:\n%s", change.Path, detail)
	}
}

// previewRoutes prints the Caddy file a change would write next to the one on
// the machine, reading it but changing nothing.
func previewRoutes(ctx context.Context, out io.Writer, dir string, cfg config.Config, machine config.Machine, workspace string) error {
	change, err := routesChange(ctx, dir, cfg, machine, workspace)
	if err != nil {
		return err
	}
	current := ""
	if client, _, err := dialMux(ctx, machine, ""); err != nil {
		fmt.Fprintf(out, "%s could not be reached (%v): showing the file against an empty one\n", machine.Name, err)
	} else {
		defer client.Close()
		if current, err = client.Run(ctx, fmt.Sprintf("cat %s 2>/dev/null || true", quoteForShell(change.Path))); err != nil {
			return fmt.Errorf("reading %s on %s: %w", change.Path, machine.Name, err)
		}
	}
	fmt.Fprintf(out, "%s on %s:\n", change.Path, machine.Name)
	if change.Content == nil {
		fmt.Fprintln(out, "(removed: the workspace has no route left)")
	}
	fmt.Fprint(out, expose.Diff(current, string(change.Content)))
	for _, stale := range change.Stale {
		fmt.Fprintf(out, "%s is removed if it is there: the old `expose` wrote it for a host this file now owns\n", stale)
	}
	return nil
}

func withRoute(cfg config.Config, workspace string, r config.Route) config.Config {
	cfg.Workspaces = slices.Clone(cfg.Workspaces)
	for i := range cfg.Workspaces {
		if cfg.Workspaces[i].Name == workspace {
			cfg.Workspaces[i].Routes = append(slices.Clone(cfg.Workspaces[i].Routes), r)
		}
	}
	return cfg
}

func withoutRoute(cfg config.Config, host string) config.Config {
	cfg.Workspaces = slices.Clone(cfg.Workspaces)
	for i := range cfg.Workspaces {
		cfg.Workspaces[i].Routes = slices.DeleteFunc(slices.Clone(cfg.Workspaces[i].Routes),
			func(r config.Route) bool { return r.Host == host })
	}
	return cfg
}

func newExposeAddCmd(opts *options) *cobra.Command {
	var host, via string
	var check, yes, publish, noApply bool

	c := &cobra.Command{
		Use:   "add <workspace> <port>",
		Short: "Publish a workspace's port as a public hostname, on Caddy at once",
		Long: "Records the route in the configuration, points the name at the " +
			"machine, and writes the workspace's routes file on the machine and " +
			"reloads Caddy — the same file sync writes, without running the " +
			"whole machine's play. A machine that cannot be reached keeps the " +
			"route pending until the next `devmachine sync`. Before recording " +
			"anything it asks — because whatever is behind the port becomes " +
			"reachable by anybody who learns the hostname.\n\n" +
			"--via publishes it through another machine's Caddy, for a workspace " +
			"on a machine the internet cannot reach: that machine proxies to the " +
			"workspace's machine over the address its `hosts` give, a private " +
			"network first.",
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
			route := config.Route{Host: host, Port: port, Via: via}
			serving := tgt.machine
			if via != "" {
				if err := withRoute(cfg, workspace, route).Validate(); err != nil {
					return err
				}
				if serving, err = cfg.Machine(via); err != nil {
					return err
				}
			}
			if _, err := caddySitesDir(cmd.Context(), dir, cfg, serving); err != nil {
				if noCaddy := (errNoCaddy{}); via == "" && errors.As(err, &noCaddy) {
					if others := servingMachines(cmd.Context(), dir, cfg); len(others) > 0 {
						return fmt.Errorf("caddy is not on %s, where %s lives: publish it through a machine that has caddy, "+
							"with `--via %s`, or add caddy to %s with `devmachine packages add caddy --machine %s`",
							serving.Name, workspace, strings.Join(others, "` or `--via "), serving.Name, serving.Name)
					}
				}
				return err
			}

			question := fmt.Sprintf(
				"Publish %s:%d as https://%s, reachable by anybody who learns that hostname?",
				workspace, port, host)
			if via != "" {
				question = fmt.Sprintf(
					"Publish %s:%d as https://%s through %s, reachable by anybody who learns that hostname?",
					workspace, port, host, via)
			}
			if check {
				if owner, _, ok := cfg.RouteOwner(host); ok {
					return fmt.Errorf("%s is already published by workspace %q: `devmachine expose rm %s` first", host, owner.Name, host)
				}
				cmd.Println("would " + question)
				cmd.Printf("%s, workspace %s, routes:\n+ %s\n", config.FileName, workspace, routeLine(route))
				if noApply {
					return nil
				}
				return previewRoutes(cmd.Context(), cmd.OutOrStdout(), dir, withRoute(cfg, workspace, route), serving, workspace)
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

			if err := config.AddRoute(dir, workspace, route); err != nil {
				return err
			}
			command := fmt.Sprintf("expose add %s %d --host %s", workspace, port, host)
			if via != "" {
				command += " --via " + via
			}
			record(opts, tgt, command, true)
			repo.AutoCommit(cmd.Context(), dir, fmt.Sprintf("chore(config): expose %s", host))

			notes := cmd.OutOrStdout()
			if opts.format == formatJSON {
				notes = cmd.ErrOrStderr()
			}

			var client remote.Client
			if c, _, err := dialMux(cmd.Context(), serving, ""); err == nil {
				client = c
				defer client.Close()
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s could not be reached (%v); the DNS record is printed to create by hand\n", serving.Name, err)
			}
			if err := pointDNSAtMachine(cmd.Context(), dir, target{machine: serving}, host, client, cmd.ErrOrStderr()); err != nil {
				return err
			}

			result := exposeResult{Host: host, Port: port, Workspace: workspace, Via: via, Status: "pending"}
			var applyErr error
			if noApply {
				result.Note = "recorded only (--no-apply)"
				fmt.Fprintf(notes, "recorded https://%s -> %s:%d in the configuration. Run `devmachine sync` to publish it on %s.\n",
					host, workspace, port, serving.Name)
			} else if applyErr = applyRoutes(cmd.Context(), client, dir, serving, workspace); applyErr == nil {
				result.Applied, result.Status = true, "published"
				fmt.Fprintf(notes, "published: https://%s -> %s:%d on %s. Caddy gets the certificate on the first "+
					"request, which can take a few seconds.\n", host, workspace, port, serving.Name)
				if via != "" {
					if note := unreachableNote(cmd.Context(), client, serving, tgt.machine, port); note != "" {
						result.Note = note
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", note)
					}
				}
			} else if pending := (errNotApplied{}); errors.As(applyErr, &pending) {
				applyErr = nil
				result.Note = pending.reason
				fmt.Fprintf(notes, "recorded https://%s -> %s:%d, pending: %s. Run `devmachine sync` to publish it on %s.\n",
					host, workspace, port, pending.reason, serving.Name)
			} else {
				result.Note = applyErr.Error()
				applyErr = fmt.Errorf("%w\nhttps://%s is recorded in the configuration and pending: fix it and run `devmachine sync`",
					applyErr, host)
			}
			if opts.format == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), result); err != nil {
					return err
				}
			}
			return applyErr
		},
	}
	c.Flags().StringVar(&host, "host", "", "the public hostname this port answers to (required)")
	c.Flags().StringVar(&via, "via", "", "publish through this machine's Caddy instead of the workspace's own")
	c.Flags().BoolVar(&check, "check", false, "show the configuration and Caddy changes, and change nothing")
	c.Flags().BoolVar(&yes, "yes", false, "skip local-write questions; never publish")
	c.Flags().BoolVar(&publish, "publish", false, "publish the site without asking")
	c.Flags().BoolVar(&noApply, "no-apply", false, "only record the route; the next sync publishes it")
	return c
}

// routeLine is a route the way config.yml holds it.
func routeLine(r config.Route) string {
	if r.Via != "" {
		return fmt.Sprintf("{host: %s, port: %d, via: %s}", r.Host, r.Port, r.Via)
	}
	return fmt.Sprintf("{host: %s, port: %d}", r.Host, r.Port)
}

// pointDNSAtMachine reuses the same dns.Choose and Upsert path `dns add`
// uses: an installed provider gets the record written, and with none
// installed, the exact record to create by hand is printed instead of
// refusing. A name that does not resolve yet fails minutes later, in Caddy's
// certificate log, where nobody is looking.
func pointDNSAtMachine(ctx context.Context, dir string, tgt target, host string, client remote.Client, out interface {
	Write([]byte) (int, error)
}) error {
	base, err := provision.Base(tgt.machine)
	if err != nil {
		return err
	}
	choice, err := dns.Choose(ctx, dir, tgt.machine.Name, base, host, "", client, out)
	if err != nil {
		return err
	}
	if err := requiresAddress(tgt.machine, "expose"); err != nil {
		return err
	}
	addresses, err := remote.Resolve(tgt.machine)
	if err != nil {
		return err
	}
	address, private := publicAddress(addresses)
	if private {
		fmt.Fprintf(out, "warning: %s has no public address in `hosts`, so %s points at %s, which only a private network reaches\n",
			tgt.machine.Name, host, address)
	}
	rec := dns.Record{Name: labelFor(host, choice.Zone), Type: "A", Value: address}
	return choice.Provider.Upsert(ctx, choice.Zone, rec)
}

// publicAddress is the address a public name should point at: the first one
// the internet can reach. A machine is often listed with its private network
// first, because that is the better way in over SSH, and a record pointing
// there answers nobody. A DNS name counts as public. With none, the first
// address is used and private says so.
func publicAddress(addresses []string) (address string, private bool) {
	for _, a := range addresses {
		ip, err := netip.ParseAddr(a)
		if err != nil || (ip.IsGlobalUnicast() && !ip.IsPrivate() && !sharedAddressSpace.Contains(ip)) {
			return a, false
		}
	}
	return addresses[0], true
}

// sharedAddressSpace is RFC 6598's 100.64.0.0/10, which Tailscale and
// carrier-grade NAT use; netip.Addr.IsPrivate does not count it.
var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

// site is one row `expose list` reports.
type site struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Workspace string `json:"workspace,omitempty"`
	// From is the machine the port is on, for a site sent here with `via`.
	From string `json:"from,omitempty"`
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
func reconcile(cfg config.Config, machine string, onMachine []expose.Site, reachable bool, upstreams map[string]string) []site {
	seen := map[string]bool{}
	var rows []site
	for _, served := range cfg.RoutesServedBy(machine) {
		w, r := served.Workspace, served.Route
		seen[r.Host] = true
		row := site{Host: r.Host, Port: r.Port, Workspace: w.Name}
		if r.Via != "" {
			row.From = w.Machine
		}
		switch {
		case !reachable:
			row.Status = "unknown"
			row.Note = "the machine could not be asked"
		default:
			found, ok := find(onMachine, r.Host)
			want, known := upstreams[row.From]
			switch {
			case !ok:
				row.Status = "pending"
				row.Note = "not on the machine yet: run `devmachine sync`"
			case found.Port != r.Port || found.Workspace != w.Name:
				row.Status = "differs"
				row.Note = fmt.Sprintf("the machine has port %d for %s: run `devmachine sync`", found.Port, orDash(found.Workspace))
			case row.From == "" && found.Upstream != "":
				row.Status = "differs"
				row.Note = fmt.Sprintf("the machine proxies to %s instead of itself: run `devmachine sync`", found.Upstream)
			case row.From != "" && found.Upstream == "":
				row.Status = "differs"
				row.Note = fmt.Sprintf("the machine proxies to itself instead of %s: run `devmachine sync`", row.From)
			case row.From != "" && known && found.Upstream != want:
				row.Status = "differs"
				row.Note = fmt.Sprintf("the machine proxies to %s, and %s is at %s: run `devmachine sync`",
					orDash(found.Upstream), row.From, want)
			default:
				row.Status = "published"
			}
		}
		rows = append(rows, row)
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

// upstreamsFor is the address of every machine that sends a site to this one,
// as sync would write it now. A machine whose address cannot be found is left
// out, and its sites are not compared on it.
func upstreamsFor(ctx context.Context, cfg config.Config, machine string) map[string]string {
	out := map[string]string{}
	for _, served := range cfg.RoutesServedBy(machine) {
		if served.Route.Via == "" {
			continue
		}
		from := served.Workspace.Machine
		if _, done := out[from]; done {
			continue
		}
		if m, err := cfg.Machine(from); err == nil {
			if address, err := upstreamOf(ctx, m); err == nil {
				out[from] = address
			}
		}
	}
	return out
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
				rows = reconcile(cfg, tgt.machine.Name, nil, false, nil)
			} else if client, _, err := dial(cmd.Context(), tgt.machine, ""); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s could not be reached (%v)\n", tgt.machine.Name, err)
				rows = reconcile(cfg, tgt.machine.Name, nil, false, nil)
			} else {
				defer client.Close()
				sites, err := listSites(cmd.Context(), client, sitesDir)
				if err != nil {
					return err
				}
				rows = reconcile(cfg, tgt.machine.Name, sites, true, upstreamsFor(cmd.Context(), cfg, tgt.machine.Name))
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), rows)
			}
			if len(rows) == 0 {
				cmd.Println("Nothing is published.")
				return nil
			}
			for _, r := range rows {
				owner := orDash(r.Workspace)
				if r.From != "" {
					owner += " (from " + r.From + ")"
				}
				cmd.Printf("%-28s %-6d %-10s %-10s %s\n", r.Host, r.Port, owner, r.Status, r.Note)
			}
			return nil
		},
	}
}

// servingTarget is the machine whose Caddy serves a host: the one the route
// names, whatever --machine says, or --machine for a host the configuration
// does not know.
func servingTarget(opts *options, cfg config.Config, host string) (target, error) {
	owner, route, ok := cfg.RouteOwner(host)
	if !ok {
		return machineTarget(opts)
	}
	m, err := cfg.Machine(cfg.ServingMachine(owner, route))
	if err != nil {
		return target{}, err
	}
	return target{machine: m}, nil
}

func newExposeRmCmd(opts *options) *cobra.Command {
	var check, yes, noApply bool

	c := &cobra.Command{
		Use:   "rm <host>",
		Short: "Stop publishing a site, on Caddy at once",
		Long: "Forgets the route in the configuration, then rewrites the " +
			"workspace's routes file on the machine without it and reloads " +
			"Caddy. A machine that cannot be reached keeps serving the site " +
			"until the next `devmachine sync`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			if !config.UsableHost(host) {
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
			tgt, err := servingTarget(opts, cfg, host)
			if err != nil {
				return err
			}
			question := fmt.Sprintf("Stop publishing https://%s ?", host)
			if check {
				cmd.Println("would " + question)
				owner, route, ok := cfg.RouteOwner(host)
				if !ok {
					return nil
				}
				cmd.Printf("%s, workspace %s, routes:\n- %s\n", config.FileName, owner.Name, routeLine(route))
				if noApply {
					return nil
				}
				if _, err := caddySitesDir(cmd.Context(), dir, cfg, tgt.machine); err != nil {
					cmd.Printf("nothing to change on %s: %v\n", tgt.machine.Name, err)
					return nil
				}
				return previewRoutes(cmd.Context(), cmd.OutOrStdout(), dir, withoutRoute(cfg, host), tgt.machine, owner.Name)
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

			notes := cmd.OutOrStdout()
			if opts.format == formatJSON {
				notes = cmd.ErrOrStderr()
			}
			result := exposeResult{Host: host, Workspace: owner, Status: "pending"}
			var applyErr error
			switch {
			case noApply:
				result.Note = "recorded only (--no-apply)"
				fmt.Fprintf(notes, "%s is no longer in the configuration (it was %s's). Run `devmachine sync` to take it off %s.\n",
					host, owner, tgt.machine.Name)
			default:
				var client remote.Client
				if _, err := caddySitesDir(cmd.Context(), dir, cfg, tgt.machine); err != nil {
					applyErr = errNotApplied{err.Error()}
				} else if c, _, err := dialMux(cmd.Context(), tgt.machine, ""); err != nil {
					applyErr = errNotApplied{fmt.Sprintf("%s could not be reached (%v)", tgt.machine.Name, err)}
				} else {
					client = c
					defer client.Close()
					applyErr = applyRoutes(cmd.Context(), client, dir, tgt.machine, owner)
				}
				if applyErr == nil {
					result.Applied, result.Status = true, "removed"
					fmt.Fprintf(notes, "https://%s is no longer published: Caddy on %s dropped it (it was %s's).\n",
						host, tgt.machine.Name, owner)
				} else if pending := (errNotApplied{}); errors.As(applyErr, &pending) {
					applyErr = nil
					result.Note = pending.reason
					fmt.Fprintf(notes, "%s is no longer in the configuration (it was %s's), but %s. "+
						"Run `devmachine sync` to take it off %s.\n", host, owner, pending.reason, tgt.machine.Name)
				} else {
					result.Note = applyErr.Error()
					applyErr = fmt.Errorf("%w\n%s is out of the configuration, and %s still serves it until `devmachine sync`",
						applyErr, host, tgt.machine.Name)
				}
			}
			if opts.format == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), result); err != nil {
					return err
				}
			}
			return applyErr
		},
	}
	c.Flags().BoolVar(&check, "check", false, "show the configuration and Caddy changes, and change nothing")
	c.Flags().BoolVar(&yes, "yes", false, "remove without asking")
	c.Flags().BoolVar(&noApply, "no-apply", false, "only forget the route; the next sync takes it off the machine")
	return c
}
