package commands

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/mydevmachine/devmachine/internal/remote"
	"github.com/spf13/cobra"
)

// Seams, so a test decides what DNS answers and what a connection reaches.
var (
	realLookupIP = lookupIPAddr
	lookupIP     = realLookupIP
	realProxy    = remote.Proxy
	proxy        = realProxy
)

// dnsTimeout bounds one name lookup in `resolve`: a slow resolver must not
// hold up a list whose other addresses are already known.
const dnsTimeout = 5 * time.Second

func lookupIPAddr(ctx context.Context, host string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()
	found, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(found))
	for _, a := range found {
		out = append(out, a.IP.String())
	}
	return out, nil
}

func newResolveCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "resolve",
		Short: "The addresses a machine is reached on, in the order they are tried",
		Long: "Turns the machine's `hosts` into IP addresses, the way every " +
			"connection does: a `<prefix>:<name>` entry is asked of the network " +
			"package that declares the prefix, and a DNS name is looked up. " +
			"Each address says which entry and which package it came from, and " +
			"every entry that gave no address says why.\n\n" +
			"It is what anything outside the CLI should call instead of guessing: " +
			"the macOS app, or a mosh client, whose UDP target has to be an IP.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResolve(cmd, opts)
		},
	}
}

func runResolve(cmd *cobra.Command, opts *options) error {
	tgt, err := machineTarget(opts)
	if err != nil {
		return err
	}
	if err := requiresAddress(tgt.machine, "resolve"); err != nil {
		return err
	}
	r := withIPs(cmd.Context(), remote.ResolveAll(cmd.Context(), tgt.machine))

	if opts.format == formatJSON {
		return writeJSON(cmd.OutOrStdout(), r)
	}
	if len(r.Addresses) == 0 {
		cmd.Printf("%s has no address that works from here.\n", r.Machine)
	} else {
		cmd.Printf("%-40s %-28s %s\n", "ADDRESS", "FROM", "PACKAGE")
		for _, a := range r.Addresses {
			cmd.Printf("%-40s %-28s %s\n", a.Address, a.Source, orDash(a.Package))
		}
	}
	if len(r.Dropped) > 0 {
		cmd.Println("\nskipped:")
		for _, d := range r.Dropped {
			cmd.Printf("  %-28s %s\n", d.Source, d.Reason)
		}
	}
	return nil
}

// withIPs looks up every DNS name a literal entry holds, so each address is
// an IP: what the macOS app and a mosh client need. A name DNS cannot answer
// is skipped the same way a network that is off is.
func withIPs(ctx context.Context, r remote.Resolution) remote.Resolution {
	out := remote.Resolution{Machine: r.Machine, Addresses: []remote.Address{}, Dropped: r.Dropped}
	seen := map[string]bool{}
	add := func(a remote.Address) {
		if !seen[a.Address] {
			seen[a.Address] = true
			out.Addresses = append(out.Addresses, a)
		}
	}
	for _, a := range r.Addresses {
		if net.ParseIP(a.Address) != nil {
			add(a)
			continue
		}
		ips, err := lookupIP(ctx, a.Address)
		if err != nil {
			out.Dropped = append(out.Dropped, remote.Dropped{Source: a.Source, Reason: err.Error()})
			continue
		}
		for _, ip := range ips {
			add(remote.Address{Address: ip, Source: a.Source, Package: a.Package})
		}
	}
	return out
}

func newSSHProxyCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "ssh-proxy <machine> <port>",
		Short: "Connect ssh to a machine through its first address that answers",
		Long: "The ProxyCommand the SSH aliases use. It resolves the machine's " +
			"addresses the moment ssh connects, tries them in order, and pipes " +
			"the first connection that opens between ssh and the machine.\n\n" +
			"Nothing but the connection goes to standard output, so it is not " +
			"meant to be run by hand.",
		Hidden: true,
		Args:   cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSSHProxy(cmd, opts, args[0], args[1])
		},
	}
}

func runSSHProxy(cmd *cobra.Command, opts *options, machine, port string) error {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("port %q is not a TCP port", port)
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}
	m, err := cfg.Machine(machine)
	if err != nil {
		return err
	}
	if err := requiresAddress(m, "ssh-proxy"); err != nil {
		return err
	}
	addresses, err := remote.Resolve(m)
	if err != nil {
		return err
	}
	targets := make([]string, 0, len(addresses))
	for _, a := range addresses {
		targets = append(targets, net.JoinHostPort(a, port))
	}
	_, err = proxy(cmd.Context(), targets, cmd.InOrStdin(), cmd.OutOrStdout())
	if err != nil {
		return fmt.Errorf("machine %q: %w", m.Name, err)
	}
	return nil
}
