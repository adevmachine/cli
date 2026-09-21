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
	"path/filepath"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/remote"
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
// whether this computer is on a tailnet right now.
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
	// IdentityFile is empty when the SSH agent holds the key, which is how a
	// key kept in a password manager works: there is no file to point at.
	IdentityFile string `json:"identity_file,omitempty"`
	// HostKeyAlias is the same for every alias of one machine. known_hosts is
	// indexed by address, so a machine on two addresses gets two entries and
	// switching between them gives "Host key verification failed". This
	// indexes by name instead.
	HostKeyAlias string `json:"host_key_alias"`
}

// List returns the aliases a configuration describes, workspace by workspace.
func List(cfg config.Config) ([]Alias, error) {
	var out []Alias

	for _, w := range cfg.Workspaces {
		machine, err := cfg.Machine(w.Machine)
		if err != nil {
			return nil, err
		}
		addresses, err := resolve(machine)
		if err != nil || len(addresses) == 0 {
			return nil, errNoAddress(machine.Name)
		}

		entry := Alias{
			Name: w.Name + suffix, Host: addresses[0], User: w.LinuxUser(),
			Port: machine.Port, IdentityFile: machine.Key,
			HostKeyAlias: machine.Name + suffix,
		}
		out = append(out, entry)

		// A `-pub` alias is the way back in when the first path is down, so
		// it only earns its place when it points somewhere else. With one
		// address, or with the tailnet already down and the primary resolved
		// to the public address, it would only repeat the entry above it.
		if fallback := literalAddress(machine); fallback != "" && fallback != entry.Host {
			pub := entry
			pub.Name = w.Name + suffix + "-pub"
			pub.Host = fallback
			out = append(out, pub)
		}
	}
	return out, nil
}

// literalAddress is the first host entry ssh can dial as written.
func literalAddress(m config.Machine) string {
	for _, h := range m.Hosts {
		if remote.Literal(h.Address) {
			return h.Address
		}
	}
	return ""
}

// Render returns the body of the managed block: the Host entries themselves,
// without the markers Write puts around them.
func Render(cfg config.Config) (string, error) {
	found, err := List(cfg)
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
		if a.IdentityFile != "" {
			fmt.Fprintf(&b, "    IdentityFile %s\n", a.IdentityFile)
			// Without this, ssh offers every key the agent holds first, and a
			// server can cut the connection at MaxAuthTries before the one
			// that works is ever tried.
			fmt.Fprintf(&b, "    IdentitiesOnly yes\n")
		}
		fmt.Fprintf(&b, "    HostKeyAlias %s\n", a.HostKeyAlias)
	}
	return b.String(), nil
}

// Write replaces the managed block in a file and leaves everything else alone.
//
// An SSH configuration holds hosts this CLI knows nothing about — a work jump
// host, a sandbox, a client's bastion. Rewriting the whole file deletes them,
// and nothing brings them back.
func Write(path, block string) error {
	body, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		body = nil
	case err != nil:
		return fmt.Errorf("reading %s: %w", path, err)
	}

	managed := Begin + "\n" + strings.TrimRight(block, "\n") + "\n" + End + "\n"

	var out string
	current := string(body)
	start := strings.Index(current, Begin)
	stop := strings.LastIndex(current, End)
	switch {
	case start >= 0 && stop > start:
		out = current[:start] + managed
		if rest := strings.TrimLeft(current[stop+len(End):], "\n"); rest != "" {
			out += "\n" + rest
		}
	case current == "":
		out = managed
	default:
		out = strings.TrimRight(current, "\n") + "\n\n" + managed
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	// ssh refuses to read a configuration other people can write, so a file
	// this makes starts at 0600 and one that exists keeps what it had.
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, []byte(out), mode); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// DefaultPath is the person's own SSH configuration.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".ssh", "config"), nil
}
