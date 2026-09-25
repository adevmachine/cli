package commands

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/hostkeys"
	"github.com/adevmachine/cli/internal/keys"
	"github.com/adevmachine/cli/internal/remote"
	agentskills "github.com/adevmachine/cli/internal/skills"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// The bootstrap's steps, as seams, so every branch of the flow can be driven
// without a machine to reach.
var (
	dialWith       = remote.DialWith
	installKey     = remote.InstallKey
	proveAuth      = remote.ProveAuth
	harden         = remote.Harden
	installAnsible = remote.InstallAnsible
	agentKeys      = keys.FromAgent
	scanHostKey    = remote.ScanHostKey
	confirmHostKey = confirm
	setupSkills    = offerSetupSkills
)

// setupOptions are the flags the flow reads.
type setupOptions struct {
	force    bool
	noHarden bool
	machine  string
}

func newSetupCmd(opts *options) *cobra.Command {
	var s setupOptions

	c := &cobra.Command{
		Use:   "setup",
		Short: "Take over a new machine or prepare the configured one",
		Long: "With no configuration, asks where the machine is and how to log in to it, then makes it " +
			"the CLI's own: it installs a key, proves the key on a connection of " +
			"its own, turns password login off, and installs Ansible. With an existing " +
			"configuration, it reuses the configured trust and authentication and only " +
			"ensures Ansible is installed; --force explicitly starts over.\n\n" +
			"It never asks which situation you are in. A key that already works " +
			"is found by trying it, and the root password is asked for only when " +
			"that fails — many servers arrive with a key already pasted in, and " +
			"their owner has no password to give.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			s.machine = opts.machine
			return runSetup(cmd.Context(), dir, cmd.InOrStdin(), cmd.OutOrStdout(), s)
		},
	}
	c.Flags().BoolVar(&s.force, "force", false, "overwrite a configuration that already exists")
	c.Flags().BoolVar(&s.noHarden, "no-harden", false,
		"during a new takeover, leave password login on (the key is still installed and proved)")
	c.AddCommand(newSetupGitCmd(opts))
	return c
}

// runSetup asks the questions, writes config.yml, and takes the machine over.
//
// It reads and writes through the streams it is given, so the whole flow is
// testable without a terminal.
func runSetup(ctx context.Context, dir string, in io.Reader, out io.Writer, opts setupOptions) error {
	path := filepath.Join(dir, config.FileName)
	if _, err := os.Stat(path); err == nil && !opts.force {
		if err := prepareExisting(ctx, dir, out, opts.machine); err != nil {
			return err
		}
		if err := finishSetup(ctx, dir, in, out); err != nil {
			return err
		}
		fmt.Fprintf(out, "\nNext: `devmachine doctor`, then `devmachine sync`.\n")
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking %s: %w", path, err)
	}

	r := bufio.NewReader(in)

	m, err := askForMachine(r, out, "main")
	if err != nil {
		return err
	}
	domain, err := ask(r, out, "domain (leave empty for none)", "")
	if err != nil {
		return err
	}

	cfg := config.Config{Machines: []config.Machine{m}, Domain: domain}
	if err := cfg.Validate(); err != nil {
		return err
	}
	m.KnownHostsFile = filepath.Join(dir, config.KnownHostsFileName)
	if err := trustFirstContact(ctx, r, out, m); err != nil {
		return err
	}

	key, err := askForKey(r, out, dir, m.Name)
	if err != nil {
		return err
	}
	m.Key = key.Path

	if err := writeConfig(dir, path, configFile{
		Machines: []machineFile{machineEntry(m)},
		Defaults: defaultsFile{Workspace: config.DefaultWorkspacePackages},
		Domain:   domain,
	}); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nwrote %s\n\n", path)

	if err := bootstrap(ctx, r, in, out, m, key, opts.noHarden); err != nil {
		return err
	}
	if err := finishSetup(ctx, dir, in, out); err != nil {
		return err
	}

	fmt.Fprintf(out, "\nNext: `devmachine doctor`, then `devmachine sync`.\n")
	return nil
}

// prepareExisting resumes setup without taking ownership a second time. The
// configuration already says which key and host identity to trust, so this
// path only connects with those choices and installs the last prerequisite.
// In particular, it must not rewrite configuration, install a key, or change
// SSH policy on a machine the CLI already owns.
func prepareExisting(ctx context.Context, dir string, out io.Writer, name string) error {
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	m, err := cfg.Machine(name)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "%s already describes %s; preparing it without rewriting configuration.\n", config.FileName, m.Name)
	client, address, err := dial(ctx, m, m.User)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	fmt.Fprintf(out, "connected as %s@%s; installing Ansible if needed...\n", m.User, address)
	if err := installAnsible(ctx, client, out); err != nil {
		return err
	}
	return nil
}

func finishSetup(ctx context.Context, dir string, in io.Reader, out io.Writer) error {
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	if cfg.Packages == "" {
		fmt.Fprintln(out, "\nAgent skills: no package release is pinned. Pin `packages:` in config.yml, then run `devmachine skills add`.")
		return nil
	}
	return setupSkills(ctx, dir, in, out)
}

func offerSetupSkills(ctx context.Context, dir string, in io.Reader, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	agents := detectAgents(home)
	if len(agents) == 0 {
		fmt.Fprintln(out, "\nAgent skills were not installed because no supported harness was detected. Run `devmachine skills add --agent <name>` later.")
		return nil
	}
	ok, err := confirm(in, out, "Install the official Devmachine skills for "+joinAgents(agents)+"?")
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out, "Agent skills were not installed. Run `devmachine skills add` later.")
		return nil
	}
	source, err := skillSource(ctx, dir, "")
	if err != nil {
		return err
	}
	result, err := (agentskills.Installer{Home: home}).Install(source, agents)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "installed %s from %s (%d filesystem changes)\n", strings.Join(result.Skills, ", "), result.Source, result.Changed)
	return nil
}

// trustFirstContact records the host identity before setup offers any
// authentication method or changes the remote machine.
func trustFirstContact(ctx context.Context, reader io.Reader, out io.Writer, machine config.Machine) error {
	presented, address, err := scanHostKey(ctx, machine)
	if err != nil {
		return err
	}
	fingerprint := hostkeys.Fingerprint(presented)
	fmt.Fprintf(out, "\n%s at %s presented %s host key %s.\n", machine.Name, address, presented.Type(), fingerprint)

	store, err := hostkeys.Open(machine.KnownHostsFile)
	if err != nil {
		return err
	}
	current, err := store.Key(machine.Name, machine.Port)
	if err == nil {
		if bytes.Equal(current.Marshal(), presented.Marshal()) {
			fmt.Fprintf(out, "the presented host key matches the trusted key in %s\n", machine.KnownHostsFile)
			return nil
		}
		return fmt.Errorf("%w for machine %q at %s: expected %s, received %s; use `devmachine machines trust %s --replace` only after verifying a deliberate rebuild",
			remote.ErrHostKeyChanged, machine.Name, address, hostkeys.Fingerprint(current), fingerprint, machine.Name)
	}
	var keyErr *knownhosts.KeyError
	if !errors.As(err, &keyErr) || len(keyErr.Want) != 0 {
		return err
	}

	question := fmt.Sprintf("Trust this %s fingerprint for %s? This is trust on first use; compare it through the provider console or another trusted channel first.", fingerprint, machine.Name)
	ok, err := confirmHostKey(reader, out, question)
	if err != nil {
		return err
	}
	if !ok {
		return errDeclined
	}
	if err := os.MkdirAll(filepath.Dir(machine.KnownHostsFile), 0o700); err != nil {
		return fmt.Errorf("creating the configuration directory for host trust: %w", err)
	}
	if err := store.Put(machine.Name, machine.Port, presented); err != nil {
		return err
	}
	fmt.Fprintf(out, "trusted %s in %s\n", fingerprint, machine.KnownHostsFile)
	return nil
}

// askForMachine asks where the machine is and who to log in as. The domain is
// not asked for here: it belongs to the configuration, not to a machine, so
// only setup has a reason to ask.
func askForMachine(r *bufio.Reader, out io.Writer, defaultName string) (config.Machine, error) {
	var m config.Machine

	name, err := ask(r, out, "machine name", defaultName)
	if err != nil {
		return m, err
	}
	address, err := ask(r, out, "address (an IP, a hostname, or tailscale:<name>)", "")
	if err != nil {
		return m, err
	}
	if address == "" {
		return m, errors.New("an address is required: the CLI has nothing to reach without one")
	}
	admin, err := ask(r, out, "administrative login", config.DefaultAdminUser)
	if err != nil {
		return m, err
	}
	portText, err := ask(r, out, "SSH port", strconv.Itoa(config.DefaultPort))
	if err != nil {
		return m, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return m, fmt.Errorf("%q is not a port number", portText)
	}

	return config.Machine{
		Name:  name,
		Hosts: []config.Host{{Address: address}},
		User:  admin,
		Port:  port,
	}, nil
}

// chosenKey is how the CLI will log in from now on.
type chosenKey struct {
	// Path is the private key on disk. It is empty when an SSH agent holds
	// the key and nothing on disk does.
	Path string
	// Public is the authorized_keys line, which is what gets installed.
	Public string
}

func (k chosenKey) auth() remote.Auth {
	if k.Path == "" {
		return remote.Auth{Agent: true}
	}
	return remote.Auth{KeyPath: k.Path}
}

func (k chosenKey) describe() string {
	if k.Path == "" {
		return "the key from the SSH agent"
	}
	return "the key " + k.Path
}

// askForKey offers the three ways in: a key of the CLI's own, a key file, or
// one the agent already holds.
func askForKey(r *bufio.Reader, out io.Writer, dir, machine string) (chosenKey, error) {
	held, err := agentKeys()
	if err != nil {
		// An agent that answers and will not talk is worth saying out loud,
		// and is not a reason to stop: the other two ways in still work.
		fmt.Fprintf(out, "\nthe SSH agent could not be read (%v), so its keys are not offered\n", err)
		held = nil
	}

	mine := filepath.Join(keys.Dir(dir), machine)
	_, err = os.Stat(mine)
	reuse := err == nil

	fmt.Fprintf(out, "\nHow should the CLI log in to this machine?\n")
	if reuse {
		fmt.Fprintf(out, "  1) use the key it already made at %s (recommended)\n", mine)
	} else {
		fmt.Fprintf(out, "  1) make a key of its own, used for nothing else (recommended)\n")
	}
	fmt.Fprintf(out, "  2) use a key file already on this computer\n")
	for i, k := range held {
		fmt.Fprintf(out, "  %d) %s  %s  (from the SSH agent)\n", i+3, k.Fingerprint, k.Comment)
	}

	answer, err := ask(r, out, "choice", "1")
	if err != nil {
		return chosenKey{}, err
	}
	choice, err := strconv.Atoi(answer)
	if err != nil || choice < 1 || choice > 2+len(held) {
		return chosenKey{}, fmt.Errorf("%q is not one of the choices", answer)
	}

	switch choice {
	case 1:
		if reuse {
			public, err := keys.PublicFor(mine)
			if err != nil {
				return chosenKey{}, err
			}
			return chosenKey{Path: mine, Public: public}, nil
		}
		path, public, err := keys.Generate(keys.Dir(dir), machine)
		if err != nil {
			return chosenKey{}, err
		}
		fmt.Fprintf(out, "made %s\n", path)
		return chosenKey{Path: path, Public: public}, nil

	case 2:
		path, err := ask(r, out, "path to the private key", "")
		if err != nil {
			return chosenKey{}, err
		}
		path, err = expandHome(path)
		if err != nil {
			return chosenKey{}, err
		}
		if path == "" {
			return chosenKey{}, errors.New("no path was given")
		}
		public, err := keys.PublicFor(path)
		if err != nil {
			return chosenKey{}, err
		}
		return chosenKey{Path: path, Public: public}, nil
	}

	// An agent key has no path. Leaving `key` out of the configuration is
	// what makes the CLI ask the agent every time from then on.
	return chosenKey{Public: held[choice-3].PublicKey}, nil
}

// bootstrap gets in, makes sure the key works, and shuts the password door.
//
// The order is the whole point and is not negotiable: prove the key on a
// connection of its own before turning password login off. The other way round
// is locking the door with the key still inside.
func bootstrap(ctx context.Context, r *bufio.Reader, source io.Reader, out io.Writer,
	m config.Machine, key chosenKey, noHarden bool) error {
	client, address, err := dialWith(ctx, m, m.User, key.auth())
	switch {
	case err == nil:
		fmt.Fprintf(out, "%s already logs in as %s@%s, so no password is needed.\n",
			key.describe(), m.User, address)
	case !errors.Is(err, remote.ErrAuthRefused):
		// Nothing answered. A machine nobody can reach is not one whose
		// password is worth asking for.
		return err
	default:
		client, err = installWithPassword(ctx, r, source, out, m, key)
		if err != nil {
			return err
		}
	}
	defer func() { _ = client.Close() }()

	if noHarden {
		fmt.Fprintf(out, "--no-harden: password login is left as it was.\n")
	} else {
		fmt.Fprintf(out, "turning password login off...\n")
		if err := harden(ctx, client); err != nil {
			return err
		}
		fmt.Fprintf(out, "password login is off; the key is the only way in.\n")
	}

	// The last thing done by hand. From here on everything is a play.
	fmt.Fprintf(out, "installing Ansible...\n")
	return installAnsible(ctx, client, out)
}

// installWithPassword is the branch for a machine as it was bought: a root
// password and nothing else. It returns the connection that proved the key.
func installWithPassword(ctx context.Context, r *bufio.Reader, source io.Reader, out io.Writer,
	m config.Machine, key chosenKey) (remote.Client, error) {
	address := m.Hosts[0].Address
	fmt.Fprintf(out, "%s does not log in yet, so the password is needed once to install it.\n", key.describe())

	password, err := askPassword(r, source, out, fmt.Sprintf("password for %s@%s", m.User, address))
	if err != nil {
		return nil, err
	}
	if password == "" {
		return nil, errors.New("no password was given, and there is no other way in yet")
	}

	client, _, err := dialWith(ctx, m, m.User, remote.Auth{Password: password})
	if err != nil {
		return nil, err
	}
	err = installKey(ctx, client, key.Public)
	// The password's whole life ends here: one connection, one use, nothing
	// written down.
	_ = client.Close()
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "the key is installed; proving it on a connection of its own...\n")

	proved, err := proveAuth(ctx, m, m.User, key.auth())
	if err != nil {
		// The detail is in the error the caller prints. What goes here is the
		// one thing somebody needs to know before they panic: the machine is
		// still reachable the way they reached it a minute ago.
		fmt.Fprintf(out,
			"\nThe key was installed and does not log in, so nothing was hardened and "+
				"password login is still on: you can still get in with the password.\n")
		return nil, fmt.Errorf("the key was installed but not proved, so password login was left on: %w", err)
	}
	fmt.Fprintf(out, "the key works.\n")
	return proved, nil
}

// askPassword reads a password without echoing it when there is a terminal to
// turn the echo off on, and plainly when there is not.
//
// Deciding from the stream rather than from os.Stdin is what makes every
// branch of this flow testable, and it is also correct: a caller piping
// answers in has no terminal to hide anything from.
func askPassword(r *bufio.Reader, source io.Reader, out io.Writer, question string) (string, error) {
	fmt.Fprintf(out, "%s: ", question)

	if f, ok := source.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		body, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("reading the password: %w", err)
		}
		return string(body), nil
	}

	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		return "", fmt.Errorf("reading the password: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// expandHome turns a leading ~ into the home directory, because that is how
// people write the path to their own key.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}

// writeConfig puts the file on disk, readable by its owner and nobody else.
func writeConfig(dir, path string, file configFile) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	body, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("rendering the configuration: %w", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// machineEntry is a machine as the file holds it.
func machineEntry(m config.Machine) machineFile {
	addresses := make([]string, 0, len(m.Hosts))
	for _, h := range m.Hosts {
		addresses = append(addresses, h.Address)
	}
	return machineFile{Name: m.Name, Hosts: addresses, User: m.User, Port: m.Port, Key: m.Key}
}

// configFile is the shape written to disk. It is separate from config.Config so
// the file keeps its intended field order and omits what was left empty.
type configFile struct {
	Machines   []machineFile   `yaml:"machines"`
	Workspaces []workspaceFile `yaml:"workspaces,omitempty"`
	Defaults   defaultsFile    `yaml:"defaults,omitempty"`
	Domain     string          `yaml:"domain,omitempty"`
}

type defaultsFile struct {
	Workspace []string `yaml:"workspace,omitempty"`
}

type machineFile struct {
	Name  string   `yaml:"name"`
	Hosts []string `yaml:"hosts"`
	User  string   `yaml:"user"`
	Port  int      `yaml:"port"`
	Key   string   `yaml:"key,omitempty"`
}

type workspaceFile struct {
	Name    string `yaml:"name"`
	Machine string `yaml:"machine,omitempty"`
	User    string `yaml:"user,omitempty"`
}

// ask prints a question and reads one line. An empty answer takes the default.
func ask(r *bufio.Reader, out io.Writer, question, fallback string) (string, error) {
	if fallback != "" {
		fmt.Fprintf(out, "%s [%s]: ", question, fallback)
	} else {
		fmt.Fprintf(out, "%s: ", question)
	}

	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return fallback, nil
		}
		return "", fmt.Errorf("reading the answer: %w", err)
	}

	answer := strings.TrimSpace(line)
	if answer == "" {
		return fallback, nil
	}
	return answer, nil
}
