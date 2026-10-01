// Package aliases writes the SSH host entries a workspace is reached by.
//
// `mosh alice-devmachine` is the daily workflow, and it needs nothing on the
// machine: the configuration already holds every workspace, its machine, its
// addresses, its port and its key. So this is a local file and no package.
package aliases

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/hostkeys"
	"github.com/mydevmachine/devmachine/internal/remote"
)

// The block's markers. Everything between them belongs to the CLI, and
// everything outside them belongs to the person whose file it is.
const (
	Begin = "# >>> devmachine — generated, do not edit"
	End   = "# <<< devmachine"
)

// suffix is what an alias is named after, so the names read the same way
// whatever the machine is called.
const suffix = "-devmachine"

// resolve is the seam a test replaces, so what gets written does not depend on
// whether your computer is on a tailnet right now.
var resolve = remote.Resolve

func errNoAddress(machine string) error {
	return fmt.Errorf("machine %q has no address left to write an alias for", machine)
}

// Alias is one Host entry.
type Alias struct {
	Name string `json:"name"`
	Host string `json:"host"`
	User string `json:"user"`
	Port int    `json:"port"`
	// IdentityFile is empty when the SSH agent offers every key it holds:
	// there is no file naming just one. It holds the machine's own key when
	// there is one, or the recorded agent key's public half otherwise.
	IdentityFile string `json:"identity_file,omitempty"`
	// HostKeyAlias is the same for every alias of one machine. known_hosts is
	// indexed by address, so a machine on two addresses gets two entries and
	// switching between them gives "Host key verification failed". This
	// indexes by name instead.
	HostKeyAlias       string `json:"host_key_alias"`
	UserKnownHostsFile string `json:"user_known_hosts_file"`
	HostKeyAlgorithms  string `json:"host_key_algorithms"`
	// ProxyCommand is set in the proxy form: ssh asks the CLI for the
	// machine's addresses each time it connects, and Host is then the
	// machine's name rather than an address.
	ProxyCommand string `json:"proxy_command,omitempty"`
}

// Options choose the form the aliases are written in.
type Options struct {
	// CLI is the devmachine executable ssh runs as the ProxyCommand. Empty
	// writes the literal form: HostName is the address resolved now.
	CLI string
}

// CLIPath is the devmachine on PATH, as an absolute path, or empty when there
// is none. It is the path the proxy form writes, so ssh finds the CLI even
// from an editor or an app that does not share the terminal's PATH.
func CLIPath() string {
	found, err := exec.LookPath("devmachine")
	if err != nil {
		return ""
	}
	abs, err := filepath.Abs(found)
	if err != nil {
		return ""
	}
	return abs
}

// List returns the aliases a configuration describes, workspace by workspace.
//
// The proxy form resolves nothing now: the addresses are asked for when ssh
// connects, so a network going up or down never leaves a stale address in
// the file. The literal form writes the first address that works now.
func List(cfg config.Config, opts Options) ([]Alias, error) {
	var out []Alias

	for _, w := range cfg.Workspaces {
		machine, err := cfg.Machine(w.Machine)
		if err != nil {
			return nil, err
		}
		if machine.Self {
			// A self machine has no address, so there is no Host entry to
			// write. Validate already refuses a workspace on one; this is
			// the defensive skip for whatever reaches here anyway.
			continue
		}
		store, err := hostkeys.Open(machine.KnownHostsFile)
		if err != nil {
			return nil, err
		}
		key, err := store.Key(machine.Name, machine.Port)
		if err != nil {
			return nil, fmt.Errorf("reading SSH host trust for machine %q: %w", machine.Name, err)
		}

		identityFile := machine.Key
		if identityFile == "" {
			identityFile = machine.AgentKeyFile
		}
		entry := Alias{
			Name: w.Name + suffix, User: w.LinuxUser(),
			Port: machine.Port, IdentityFile: identityFile,
			HostKeyAlias:       hostkeys.Lookup(machine.Name, machine.Port),
			UserKnownHostsFile: machine.KnownHostsFile,
			HostKeyAlgorithms:  strings.Join(hostkeys.Algorithms(key), ","),
		}

		if opts.CLI != "" {
			entry.Host = machine.Name
			entry.ProxyCommand = proxyCommand(opts.CLI, machine)
			out = append(out, entry)
			// The proxy already falls back on its own, so `-pub` is only
			// the way to skip the private network on purpose.
			if len(machine.Hosts) > 1 {
				if public := literalAddress(machine, ""); public != "" {
					pub := entry
					pub.Name = w.Name + suffix + "-pub"
					pub.Host, pub.ProxyCommand = public, ""
					out = append(out, pub)
				}
			}
			continue
		}

		addresses, err := resolve(machine)
		if err != nil || len(addresses) == 0 {
			return nil, errNoAddress(machine.Name)
		}
		entry.Host = addresses[0]
		out = append(out, entry)

		// A `-pub` alias is the way back in when the first path is down, so
		// it only earns its place when it points somewhere else. With one
		// address, or with the tailnet already down and the primary resolved
		// to the public address, it would only repeat the entry above it.
		if fallback := literalAddress(machine, entry.Host); fallback != "" {
			pub := entry
			pub.Name = w.Name + suffix + "-pub"
			pub.Host = fallback
			out = append(out, pub)
		}
	}
	return out, nil
}

// proxyCommand is the line ssh runs through the shell: `%p` is ssh's own
// port token, and a literal `%` anywhere else has to be written `%%`.
func proxyCommand(cli string, m config.Machine) string {
	words := []string{shellWord(cli)}
	if m.ConfigDir != "" {
		words = append(words, "--config", shellWord(m.ConfigDir))
	}
	words = append(words, "ssh-proxy", shellWord(m.Name), "%p")
	for i, w := range words[:len(words)-1] {
		words[i] = strings.ReplaceAll(w, "%", "%%")
	}
	return strings.Join(words, " ")
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9@%_+=:,./-]+$`)

// shellWord quotes only what needs it, so the usual path reads as typed.
func shellWord(s string) string {
	if plainWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// literalAddress is the first host entry ssh can dial as written that is not
// the primary address, so it names somewhere the `-pub` alias would actually
// reach.
func literalAddress(m config.Machine, primary string) string {
	for _, h := range m.Hosts {
		if remote.Literal(h.Address) && h.Address != primary {
			return h.Address
		}
	}
	return ""
}

// Render returns the body of the managed block: the Host entries themselves,
// without the markers Write puts around them.
func Render(cfg config.Config, opts Options) (string, error) {
	found, err := List(cfg, opts)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for i, a := range found {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "Host %s\n", a.Name)
		fmt.Fprintf(&b, "    HostName %s\n", a.Host)
		fmt.Fprintf(&b, "    User %s\n", a.User)
		fmt.Fprintf(&b, "    Port %d\n", a.Port)
		if a.ProxyCommand != "" {
			fmt.Fprintf(&b, "    ProxyCommand %s\n", a.ProxyCommand)
		}
		if a.IdentityFile != "" {
			fmt.Fprintf(&b, "    IdentityFile %s\n", quoteSSHConfig(a.IdentityFile))
			// Without this, ssh offers every key the agent holds first, and a
			// server can cut the connection at MaxAuthTries before the one
			// that works is ever tried.
			fmt.Fprintf(&b, "    IdentitiesOnly yes\n")
		}
		fmt.Fprintf(&b, "    HostKeyAlias %s\n", a.HostKeyAlias)
		fmt.Fprintf(&b, "    StrictHostKeyChecking yes\n")
		fmt.Fprintf(&b, "    UserKnownHostsFile %s\n", quoteSSHConfig(a.UserKnownHostsFile))
		fmt.Fprintf(&b, "    GlobalKnownHostsFile /dev/null\n")
		fmt.Fprintf(&b, "    UpdateHostKeys no\n")
		fmt.Fprintf(&b, "    CheckHostIP no\n")
		fmt.Fprintf(&b, "    VerifyHostKeyDNS no\n")
		fmt.Fprintf(&b, "    KnownHostsCommand none\n")
		fmt.Fprintf(&b, "    HostKeyAlgorithms %s\n", a.HostKeyAlgorithms)
	}
	return b.String(), nil
}

// Write replaces the managed block in a file and leaves everything else alone.
//
// An SSH configuration holds hosts this CLI knows nothing about — a work jump
// host, a sandbox, a client's bastion. Rewriting the whole file deletes them,
// and nothing brings them back.
func Write(path, block string) error {
	_, err := WriteChanged(path, block)
	return err
}

// WriteChanged is Write, reporting whether the file's content actually
// changed. Automatic refreshes — after a workspace or a machine changes —
// only have something worth printing when it did.
func WriteChanged(path, block string) (bool, error) {
	body, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		body = nil
	case err != nil:
		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	managed := Begin + "\n" + strings.TrimRight(block, "\n") + "\n" + End + "\n"

	var user string
	current := string(body)
	start := strings.Index(current, Begin)
	stop := strings.LastIndex(current, End)
	switch {
	case start >= 0 && stop > start:
		user = strings.Trim(current[:start]+"\n"+current[stop+len(End):], "\n")
	default:
		user = strings.Trim(current, "\n")
	}
	out := managed
	if user != "" {
		out += "\n" + user + "\n"
	}

	if out == current {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	// ssh refuses to read a configuration other people can write, so a file
	// this makes starts at 0600 and one that exists keeps what it had.
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, []byte(out), mode); err != nil {
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	return true, nil
}

func quoteSSHConfig(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

// PathFor is the one file a configuration keeps its aliases in:
// `ssh_aliases_path` when it names one, ~/.ssh/config otherwise. Every writer
// and every check goes through here, so the aliases never end up in two files.
func PathFor(cfg config.Config) (string, error) {
	if cfg.SSHAliasesPath == "" {
		return DefaultPath()
	}
	return Resolve(cfg.SSHAliasesPath)
}

// Resolve makes a path the person typed absolute and clean, so two spellings
// of one file compare equal. A relative path is taken from the current
// directory once, here, and never again: a refresh run from somewhere else
// must not write a stray copy there.
func Resolve(path string) (string, error) {
	expanded, err := ExpandHome(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", path, err)
	}
	return abs, nil
}

// SameFile says whether two paths are one file: the same clean path, or two
// names — a symbolic link, say — for one file that exists.
func SameFile(a, b string) bool {
	if a == b {
		return true
	}
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}

// Portable writes a path under the home directory as ~/…, so config.yml
// stays right on a computer whose home is somewhere else.
func Portable(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return path
}

// ExpandHome turns a leading ~/ into the home directory, the way a person
// writes the path in config.yml or a package passes it to --path.
func ExpandHome(path string) (string, error) {
	rest, ok := strings.CutPrefix(path, "~/")
	if !ok {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, rest), nil
}

// DefaultPath is the person's own SSH configuration.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".ssh", "config"), nil
}
