package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/hostkeys"
	"github.com/mydevmachine/devmachine/internal/local"
	"github.com/mydevmachine/devmachine/internal/repo"
	"github.com/spf13/cobra"
)

func newMachinesCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machines",
		Short: "The servers this configuration knows about",
	}
	cmd.AddCommand(
		newMachinesListCmd(opts),
		newMachinesAddCmd(opts),
		newMachinesTrustCmd(opts),
		newMachinesScanCmd(opts),
		newMachinesEditCmd(opts),
		newMachinesRmCmd(opts),
		newMachinesCreateLocalCmd(opts),
		newMachinesStartCmd(),
		newMachinesStopCmd(),
		newMachinesDeleteLocalCmd(),
	)
	return cmd
}

func newMachinesListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the configured machines and the workspaces on each",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, found, err := loadConfigIfAny(opts)
			if err != nil {
				return err
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), asJSON(cfg).Machines)
			}
			if !found {
				cmd.Println("No machines yet: `devmachine setup` adds the first one.")
				return nil
			}

			for _, m := range cfg.Machines {
				addresses := make([]string, 0, len(m.Hosts))
				for _, h := range m.Hosts {
					addresses = append(addresses, h.Address)
				}
				names := make([]string, 0)
				for _, w := range cfg.WorkspacesOn(m.Name) {
					names = append(names, w.Name)
				}
				workspaces := "none"
				if len(names) > 0 {
					workspaces = strings.Join(names, ", ")
				}
				cmd.Printf("%-12s %-28s port %-6d workspaces: %s\n",
					m.Name, strings.Join(addresses, ","), m.Port, workspaces)
			}
			return nil
		},
	}
}

func newMachinesAddCmd(opts *options) *cobra.Command {
	var (
		s        setupOptions
		selfName string
	)

	c := &cobra.Command{
		Use:   "add",
		Short: "Connect another server and add it to the configuration",
		Long: "Asks the same questions as `setup`, minus the domain, and runs the " +
			"same bootstrap: it installs a key, proves the key on a connection of " +
			"its own, turns password login off, and installs Ansible.\n\n" +
			"`setup` writes the first machine. This writes every one after it, and, " +
			"with --address and no config.yml yet, the first one too, without a terminal.\n\n" +
			"--self <name> adds your computer as a machine instead: no address, no key, no " +
			"password. It only makes sure Homebrew and Ansible are on PATH.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			if selfName != "" {
				return runMachinesAddSelf(cmd.Context(), dir, cmd.OutOrStdout(), selfName)
			}
			return runMachinesAdd(cmd.Context(), dir, cmd.InOrStdin(), cmd.OutOrStdout(), s)
		},
	}
	c.Flags().BoolVar(&s.noEssentials, "no-essentials", false,
		"start the machine with no packages, instead of the essentials")
	c.Flags().BoolVar(&s.noHarden, "no-harden", false,
		"leave password login on (the key is still installed and proved)")
	c.Flags().BoolVar(&s.noAliases, "no-aliases", false,
		"do not ask about SSH host entries, and do not write them")
	c.Flags().BoolVar(&s.yes, "yes", false, "answer yes to writing SSH host entries, without asking")
	c.Flags().StringVar(&selfName, "self", "",
		"add your computer as a machine, named <name>, instead of asking for an address")
	c.Flags().StringVar(&s.address, "address", "",
		"the machine's address; with it, nothing is asked and every other answer is a flag or its default")
	c.Flags().StringVar(&s.name, "name", "", "the machine's name (needed with --address)")
	c.Flags().StringVar(&s.user, "user", config.DefaultAdminUser,
		"the admin login: root, or an account with passwordless sudo")
	c.Flags().IntVar(&s.port, "port", config.DefaultPort, "the SSH port")
	c.Flags().StringVar(&s.key, "key", "new",
		"`new` for a key of the CLI's own for this machine (made or reused), a private key file, "+
			"or agent:<SHA256 fingerprint> for a key the SSH agent holds")
	c.Flags().StringVar(&s.fingerprint, "fingerprint", "",
		"the host key fingerprint to trust on first contact (SHA256:…), checked through another channel")
	c.Flags().BoolVar(&s.tailscale, "tailscale", false, "also add the tailscale package")
	c.Flags().StringVar(&s.domain, "domain", "",
		"the domain, written only when there is no config.yml yet and add writes a new one")
	c.Flags().BoolVar(&s.passwordStdin, "password-stdin", false,
		"with --address, read the admin password from stdin, used once to install the key")
	return c
}

// runMachinesAddSelf writes a self machine into config.yml and prepares it:
// no address is asked for, because there is none to give.
func runMachinesAddSelf(ctx context.Context, dir string, out io.Writer, name string) error {
	current, err := config.Load(dir)
	if err != nil {
		return err
	}
	if name == "" {
		return errors.New("a machine needs a name: it is how every other command says which one to act on")
	}
	for _, m := range current.Machines {
		if m.Self {
			return fmt.Errorf(
				"machine %q is already `self: true`: only one machine can be your computer", m.Name)
		}
	}
	if _, err := current.Machine(name); err == nil {
		return fmt.Errorf("a machine named %q is already configured: pick another name", name)
	}

	m := config.Machine{Name: name, Self: true}
	if err := config.AddMachine(dir, m); err != nil {
		return err
	}
	repo.AutoCommit(ctx, dir, "chore(config): add machine "+name)
	fmt.Fprintf(out, "\nadded %s (your computer) to %s\n\n", name, filepath.Join(dir, config.FileName))

	if err := prepareExistingSelf(ctx, out, m); err != nil {
		return err
	}

	fmt.Fprintf(out, "\nNext: `devmachine doctor --machine %s`, then `devmachine sync --machine %s`.\n", name, name)
	return nil
}

type machineEditOptions struct {
	set   []string
	unset []string
	check bool
	yes   bool
}

func newMachinesEditCmd(opts *options) *cobra.Command {
	var e machineEditOptions

	c := &cobra.Command{
		Use:   "edit <name>",
		Short: "Change a machine's package settings",
		Long: "It edits the configuration and touches no machine. `devmachine " +
			"sync` is what applies the change.\n\n" +
			"`--set <package>.<name>=<value>` writes into the machine's " +
			"`settings:`, which is how a machine package's variables are set. " +
			"The value is read as YAML, so `[a, b]` is a list. An empty value, " +
			"or `--unset <package>.<name>`, takes the setting out again.\n\n" +
			"A setting for a package the machine does not install is refused.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMachineEdit(cmd, opts, args[0], e)
		},
	}
	c.Flags().StringArrayVar(&e.set, "set", nil, "a setting, as <package>.<name>=<value>")
	c.Flags().StringArrayVar(&e.unset, "unset", nil, "a setting to take out, as <package>.<name>")
	c.Flags().BoolVar(&e.check, "check", false, "say what would change, and change nothing")
	c.Flags().BoolVar(&e.yes, "yes", false, "do not ask")
	return c
}

func runMachineEdit(cmd *cobra.Command, opts *options, name string, e machineEditOptions) error {
	dir, _, err := config.Dir(opts.configDir)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}
	m, err := cfg.Machine(name)
	if err != nil {
		return err
	}

	settings, changes, err := editSettings(m.Settings, e.set, e.unset)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return fmt.Errorf("nothing to change on %q: pass --set or --unset", name)
	}

	// Validating the whole configuration is what refuses a setting for a
	// package the machine does not install.
	edited := cfg
	edited.Machines = slices.Clone(cfg.Machines)
	for i := range edited.Machines {
		if edited.Machines[i].Name == name {
			edited.Machines[i].Settings = settings
		}
	}
	if err := edited.Validate(); err != nil {
		return err
	}

	if e.check {
		for _, line := range changes {
			cmd.Printf("would %s\n", line)
		}
		cmd.Println("Nothing was written.")
		return nil
	}
	if !e.yes {
		ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(),
			fmt.Sprintf("On %s: %s?", name, strings.Join(changes, "; ")))
		if err != nil {
			return err
		}
		if !ok {
			return errDeclined
		}
	}

	if err := config.SetMachineSettings(dir, name, settings); err != nil {
		return err
	}
	repo.AutoCommit(cmd.Context(), dir, "chore(config): update machine "+name)
	for _, line := range changes {
		cmd.Printf("%s: %s\n", name, line)
	}
	cmd.Println("The machine is untouched until the next `devmachine sync`.")
	return nil
}

func newMachinesRmCmd(opts *options) *cobra.Command {
	var yes bool

	c := &cobra.Command{
		Use:   "rm <name>",
		Short: "Forget a machine, leaving the server running",
		Long: "Takes the machine out of the configuration and does nothing at all " +
			"to the server: it keeps running, with everything on it, and it is " +
			"still reachable by the key.\n\n" +
			"It is not `machines delete-local`, which destroys a machine on this " +
			"computer and everything on it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(),
					"Forget the machine "+args[0]+"? The server keeps running, untouched.")
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}
			if err := config.RemoveMachine(dir, args[0]); err != nil {
				return err
			}
			repo.AutoCommit(cmd.Context(), dir, "chore(config): remove machine "+args[0])
			cmd.Printf("%s is out of the configuration.\n", args[0])
			cmd.Printf("The server itself is untouched and still running: nothing on it was " +
				"changed or deleted, and the key still gets in.\n")
			cmd.Printf("`machines delete-local` is the one that destroys a machine, and only " +
				"one on your computer.\n")

			updated, err := config.Load(dir)
			if err != nil {
				return err
			}
			return refreshAliases(updated, cmd.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "forget it without asking")
	return c
}

// runMachinesAdd asks for a machine, records it, then takes it over.
//
// It reads and writes through the streams it is given, for the same reason
// setup does: every branch of the bootstrap is reachable without a terminal.
func runMachinesAdd(ctx context.Context, dir string, in io.Reader, out io.Writer, opts setupOptions) error {
	current, err := config.Load(dir)
	fresh := errors.Is(err, os.ErrNotExist)
	if fresh && !opts.unattended() {
		return fmt.Errorf("there is no %s yet: `devmachine setup` asks for the first machine, domain "+
			"included, and `machines add --address …` adds it without questions",
			filepath.Join(dir, config.FileName))
	}
	if fresh {
		current, err = config.Config{}, nil
	}
	if err != nil {
		return err
	}
	if opts.domain != "" && !fresh {
		return fmt.Errorf("--domain only goes into a new configuration, and %s already exists: "+
			"set `domain:` in it instead", filepath.Join(dir, config.FileName))
	}

	if opts.passwordStdin && !opts.unattended() {
		return errors.New("--password-stdin needs --address: without it the answers are asked on " +
			"stdin, and the password is asked there too")
	}
	var password passwordSource
	if opts.unattended() {
		given := ""
		if opts.passwordStdin {
			if given, err = readPasswordStdin(in); err != nil {
				return err
			}
		}
		password = givenPassword(given)
		// Nothing is read: an unattended run that reached a question would
		// otherwise block on a terminal nobody is watching.
		in = strings.NewReader("")
	}
	r := bufio.NewReader(in)
	if password == nil {
		password = askingPassword(r, in, out)
	}
	var m config.Machine
	if opts.unattended() {
		m = machineFromFlags(opts)
	} else if m, err = askForMachine(r, out, ""); err != nil {
		return err
	}
	if m.Name == "" {
		return errors.New("a machine needs a name: it is how every other command says which one to act on")
	}
	if _, err := current.Machine(m.Name); err == nil {
		return fmt.Errorf("a machine named %q is already configured: pick another name", m.Name)
	}
	if fresh {
		if err := (config.Config{Machines: []config.Machine{m}, Domain: opts.domain}).Validate(); err != nil {
			return err
		}
	}
	trusted := filepath.Join(dir, config.KnownHostsFileName)
	staging, err := os.MkdirTemp("", "devmachine-add-")
	if err != nil {
		return fmt.Errorf("making a place to hold the host key until the machine is added: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	// Until the bootstrap works, the host key is trusted in a file of this
	// run's own. Written to known_hosts at once, a failed run left a key
	// keyed by the name, and the next try at a corrected address or a
	// rebuilt server was refused as a changed key for a machine that is not
	// configured, which `machines trust` cannot replace.
	m.KnownHostsFile = filepath.Join(staging, config.KnownHostsFileName)
	if opts.unattended() {
		err = trustExpected(ctx, out, m, opts.fingerprint)
	} else {
		err = trustFirstContact(ctx, r, out, m)
	}
	if err != nil {
		return err
	}

	var key chosenKey
	if opts.unattended() {
		key, err = keyFromFlag(out, dir, m.Name, opts.key)
	} else {
		key, err = askForKey(r, out, dir, m.Name)
	}
	if err != nil {
		return err
	}
	if err := recordKey(dir, &m, key); err != nil {
		return err
	}
	release := current.Packages
	if fresh {
		release = pinForNewConfig(ctx, out)
	}
	m.Packages = startingPackages(ctx, dir, release, opts.noEssentials, out)

	// The machine is written only after the bootstrap proved the key. Written
	// first, a failed run left an entry behind, and running again to fix it
	// was refused as a name already configured.
	if err := bootstrap(ctx, out, m, key, opts.noHarden, password); err != nil {
		return err
	}
	if err := keepHostKey(m, trusted); err != nil {
		return err
	}
	m.KnownHostsFile = trusted
	if fresh {
		err = writeNewConfig(dir, m, release, opts.domain)
	} else {
		err = config.AddMachine(dir, m)
	}
	if err != nil {
		return err
	}
	repo.AutoCommit(ctx, dir, "chore(config): add machine "+m.Name)
	fmt.Fprintf(out, "\nadded %s to %s\n\n", m.Name, filepath.Join(dir, config.FileName))
	if err := offerSSHAliases(r, out, dir, opts.noAliases, opts.yes || opts.unattended()); err != nil {
		return err
	}
	switch {
	case opts.unattended() && opts.tailscale:
		err = addTailscale(out, dir, m.Name)
	case !opts.unattended():
		err = offerTailscale(r, out, dir, m.Name)
	}
	if err != nil {
		return err
	}

	if updated, err := config.Load(dir); err == nil {
		if err := refreshAliases(updated, out); err != nil {
			return err
		}
	}

	fmt.Fprintf(out, "\nNext: `devmachine doctor --machine %s`, then `devmachine sync --machine %s`.\n",
		m.Name, m.Name)
	return nil
}

// keepHostKey copies the machine's host key from the run's own file into the
// configuration's known_hosts, replacing whatever an earlier run left there
// under the same name.
func keepHostKey(m config.Machine, trusted string) error {
	staged, err := hostkeys.Open(m.KnownHostsFile)
	if err != nil {
		return err
	}
	key, err := staged.Key(m.Name, m.Port)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(trusted), 0o700); err != nil {
		return fmt.Errorf("creating the configuration directory for host trust: %w", err)
	}
	store, err := hostkeys.Open(trusted)
	if err != nil {
		return err
	}
	return store.Put(m.Name, m.Port, key)
}

// pinForNewConfig is the packages release a new configuration is pinned to:
// the latest, as setup pins it, or none when it cannot be found.
func pinForNewConfig(ctx context.Context, out io.Writer) string {
	release, err := latestPackagesRelease(ctx)
	if err != nil {
		fmt.Fprintf(out, "\nno packages release is pinned (%v): run `devmachine packages pin` before the first sync.\n", err)
		return ""
	}
	return release
}

// writeNewConfig writes the configuration setup would have written for this
// machine, and AGENTS.md beside it.
func writeNewConfig(dir string, m config.Machine, release, domain string) error {
	entry := machineEntry(m)
	entry.Packages = m.Packages
	if err := writeConfig(dir, filepath.Join(dir, config.FileName), configFile{
		Packages: release,
		Machines: []machineFile{entry},
		Defaults: defaultsFile{Workspace: config.DefaultWorkspacePackages},
		Domain:   domain,
	}); err != nil {
		return err
	}
	return writeAgentsFile(dir)
}

// machineFromFlags is askForMachine's answer for an unattended run.
func machineFromFlags(opts setupOptions) config.Machine {
	return config.Machine{
		Name:  opts.name,
		Hosts: []config.Host{{Address: opts.address}},
		User:  opts.user,
		Port:  opts.port,
	}
}

// createLocal is the seam a test replaces so no VM is booted.
var createLocal = local.Create

func newMachinesCreateLocalCmd(opts *options) *cobra.Command {
	var (
		add bool
		s   setupOptions
	)
	c := &cobra.Command{
		Use:   "create-local <name>",
		Short: "Create a machine on your computer, as a bought server arrives",
		Long: "Create a machine on your computer, as a bought server arrives.\n\n" +
			"It needs Lima. The machine comes up with root reachable over SSH by " +
			"password and no key installed, which is where `devmachine setup` starts.\n\n" +
			"Without --add, nothing is written to the configuration: `setup` or " +
			"`machines add` does that. With --add, it is added at once, the way " +
			"`machines add --address` adds a server.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, flag := range []string{"key", "no-essentials", "no-aliases"} {
				if !add && cmd.Flags().Changed(flag) {
					return fmt.Errorf("--%s only applies with --add", flag)
				}
			}
			if !add {
				m, err := createLocal(cmd.Context(), args[0], cmd.ErrOrStderr())
				if err != nil {
					return err
				}
				return reportLocalMachine(cmd, opts, m, false)
			}
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			m, err := createAndAddLocal(cmd.Context(), dir, cmd.ErrOrStderr(), args[0], s)
			if err != nil {
				return err
			}
			return reportLocalMachine(cmd, opts, m, true)
		},
	}
	c.Flags().BoolVar(&add, "add", false,
		"also add it to the configuration: install a key with its password, prove it, and write it")
	c.Flags().StringVar(&s.key, "key", "new",
		"with --add: `new`, a private key file, or agent:<SHA256 fingerprint>, as `machines add --key`")
	c.Flags().BoolVar(&s.noEssentials, "no-essentials", false, "with --add: start the machine with no packages")
	c.Flags().BoolVar(&s.noAliases, "no-aliases", false, "with --add: do not write SSH host entries")
	return c
}

// createAndAddLocal creates a local machine and adds it, as one step.
//
// Progress goes to out, which is stderr, so `--format json` leaves a document
// on stdout and nothing else.
func createAndAddLocal(ctx context.Context, dir string, out io.Writer, name string, s setupOptions) (config.Machine, error) {
	current, err := config.Load(dir)
	if err == nil {
		if _, err := current.Machine(name); err == nil {
			return config.Machine{}, fmt.Errorf("a machine named %q is already configured: pick another name", name)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return config.Machine{}, err
	}

	m, err := createLocal(ctx, name, out)
	if err != nil {
		return config.Machine{}, err
	}
	// The host key is trusted as it answers: this command made the VM a
	// moment ago, and it answers only on this computer's loopback.
	presented, _, err := scanHostKey(ctx, m)
	if err != nil {
		return config.Machine{}, fmt.Errorf("the local machine %q is running, and was not added: %w", name, err)
	}
	s.name, s.address, s.user, s.port = m.Name, m.Hosts[0].Address, m.User, m.Port
	s.fingerprint = hostkeys.Fingerprint(presented)
	s.passwordStdin = true
	if err := runMachinesAdd(ctx, dir, strings.NewReader(local.Password), out, s); err != nil {
		return config.Machine{}, fmt.Errorf("the local machine %q is running, and was not added: %w; "+
			"add it with `devmachine machines add --address %s --port %d` once the cause is fixed",
			name, err, m.Hosts[0].Address, m.Port)
	}
	added, err := config.Load(dir)
	if err != nil {
		return config.Machine{}, err
	}
	return added.Machine(name)
}

func newMachinesStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start a machine on your computer",
		Long: "Start a machine on your computer.\n\n" +
			"Only a local machine, created with `create-local`: a bought server " +
			"is not the CLI's to switch on.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := local.Start(cmd.Context(), args[0]); err != nil {
				return err
			}
			cmd.Printf("%s is running\n", args[0])
			return nil
		},
	}
}

func newMachinesStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a machine on your computer",
		Long: "Stop a machine on your computer, keeping its disk.\n\n" +
			"Only a local machine, created with `create-local`: a bought server " +
			"is not the CLI's to switch off.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := local.Stop(cmd.Context(), args[0]); err != nil {
				return err
			}
			cmd.Printf("%s is stopped\n", args[0])
			return nil
		},
	}
}

func newMachinesDeleteLocalCmd() *cobra.Command {
	var yes bool

	c := &cobra.Command{
		Use:   "delete-local <name>",
		Short: "Destroy a machine on your computer",
		Long: "Destroy a machine on your computer, and everything on it.\n\n" +
			"Only a local machine, created with `create-local`. This is the one " +
			"that destroys: `machines rm` only forgets a server, and leaves it " +
			"running.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(),
					"Destroy the local machine "+args[0]+" and everything on it?")
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}
			if err := local.Delete(cmd.Context(), args[0]); err != nil {
				return err
			}
			cmd.Printf("%s is gone\n", args[0])
			return nil
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "destroy it without asking")
	return c
}

// reportLocalMachine prints a new local machine the way `machines list` prints
// a configured one, so the same fields mean the same thing.
func reportLocalMachine(cmd *cobra.Command, opts *options, m config.Machine, added bool) error {
	addresses := make([]string, 0, len(m.Hosts))
	for _, h := range m.Hosts {
		addresses = append(addresses, h.Address)
	}

	if opts.format == formatJSON {
		return writeJSON(cmd.OutOrStdout(), machineJSON{
			Name: m.Name, Hosts: addresses, AdminUser: m.User,
			Port: m.Port, Key: m.Key, AgentKey: m.AgentKey, Workspaces: []string{}, Packages: onOrNone(m.Packages),
		})
	}

	cmd.Printf("%-12s %-28s port %-6d admin: %s\n",
		m.Name, strings.Join(addresses, ","), m.Port, m.User)
	if added {
		cmd.Printf("It is in the configuration, and logs in with %s.\n", chosenKey{Path: m.Key, Public: m.AgentKey}.describe())
		return nil
	}
	cmd.Printf("It has no key on it yet, and the root password is %q.\n", local.Password)
	cmd.Printf("Set it up with `devmachine setup` (or `devmachine machines add` if you already have a machine).\n")
	cmd.Printf("Next time, `create-local %s --add` does both in one step.\n", m.Name)
	return nil
}
