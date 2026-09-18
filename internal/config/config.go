// Package config resolves, reads and validates the user's configuration.
//
// The configuration is the user's, not the CLI's: it lives in a directory they
// choose and this repository never holds a copy of it.
//
// Two ideas carry the model.
//
// A machine is a server the CLI can reach. There can be several, and each has
// its own addresses, admin login and key.
//
// A workspace is an environment: normally one Linux user on one machine. It is
// what a person works in, and what they name in a command. Where it runs is a
// property of the workspace, so `devmachine ssh a8c` needs no address and no
// machine: the mapping already says which server that is.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the configuration file inside the configuration directory.
const FileName = "config.yml"

// EnvVar overrides the configuration directory.
const EnvVar = "DEVMACHINE_CONFIG"

// Source says which rule chose the configuration directory.
type Source string

// The rules, in the order they are tried.
const (
	SourceFlag    Source = "flag"
	SourceEnv     Source = "env"
	SourceXDG     Source = "xdg"
	SourceDefault Source = "default"
)

// Defaults applied when the configuration leaves a field out.
const (
	DefaultAdminUser = "root"
	DefaultPort      = 22
)

// Host is one address a machine answers on.
//
// An address is either a literal (an IP, or a name DNS resolves) or a
// `tailscale:<machine>` entry, which something closer to the network resolves.
// Parsing an address is not this package's job; keeping the order is.
type Host struct {
	Address string
}

// UnmarshalYAML lets a host be written as a bare string.
func (h *Host) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	h.Address = s
	return nil
}

// Machine is a server the CLI operates.
type Machine struct {
	Name string `yaml:"name"`
	// Hosts are tried in order, so the first is the preferred path and the
	// rest are fallbacks.
	Hosts []Host `yaml:"hosts"`
	// User is the administrative login used to provision, not a workspace.
	User string `yaml:"user"`
	Port int    `yaml:"port"`
	// Key is a private key on disk. Left empty, the SSH agent serves the keys
	// instead, which is how a 1Password-style agent is supported without this
	// package knowing such a thing exists.
	Key string `yaml:"key"`
}

// Workspace is an environment: one Linux user on one machine.
type Workspace struct {
	Name string `yaml:"name"`
	// Machine names where this workspace lives. It may be left out when the
	// configuration has exactly one machine.
	Machine string `yaml:"machine"`
	// User overrides the Linux account. It is only needed when the workspace
	// name cannot be the account name.
	User string `yaml:"user"`
}

// LinuxUser is the account this workspace owns on its machine.
func (w Workspace) LinuxUser() string {
	if w.User != "" {
		return w.User
	}
	return w.Name
}

// Config is what config.yml holds.
type Config struct {
	Machines    []Machine   `yaml:"machines"`
	Workspaces  []Workspace `yaml:"workspaces"`
	Domain      string      `yaml:"domain"`
	DNSProvider string      `yaml:"dns_provider"`
}

// Dir returns the configuration directory and the rule that chose it.
//
// The rules are tried in order: the flag, then EnvVar, then XDG_CONFIG_HOME,
// then the home directory. Reporting the source matters as much as the path —
// "which config am I actually using" is the first question when something is
// wrong.
func Dir(flag string) (string, Source, error) {
	if flag != "" {
		return flag, SourceFlag, nil
	}
	if v := os.Getenv(EnvVar); v != "" {
		return v, SourceEnv, nil
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "devmachine"), SourceXDG, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".config", "devmachine"), SourceDefault, nil
}

// Load reads config.yml from dir and fills in the defaults.
func Load(dir string) (Config, error) {
	var c Config

	path := filepath.Join(dir, FileName)
	body, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yaml.Unmarshal(body, &c); err != nil {
		return c, fmt.Errorf("parsing %s: %w", path, err)
	}

	for i := range c.Machines {
		if c.Machines[i].User == "" {
			c.Machines[i].User = DefaultAdminUser
		}
		if c.Machines[i].Port == 0 {
			c.Machines[i].Port = DefaultPort
		}
	}
	return c, nil
}

// Machine returns the machine with that name.
//
// An empty name means "the obvious one", which only exists when there is a
// single machine. With several, this asks rather than guesses: guessing is how
// a command lands on a server nobody named.
func (c Config) Machine(name string) (Machine, error) {
	if name == "" {
		switch len(c.Machines) {
		case 0:
			return Machine{}, errors.New("no machine configured: add one under `machines` in " + FileName)
		case 1:
			return c.Machines[0], nil
		default:
			return Machine{}, fmt.Errorf(
				"several machines are configured (%s): say which one with --machine",
				strings.Join(c.machineNames(), ", "))
		}
	}
	for _, m := range c.Machines {
		if m.Name == name {
			return m, nil
		}
	}
	return Machine{}, fmt.Errorf("no machine named %q: the configured ones are %s",
		name, strings.Join(c.machineNames(), ", "))
}

// Workspace returns the workspace with that name.
func (c Config) Workspace(name string) (Workspace, error) {
	for _, w := range c.Workspaces {
		if w.Name == name {
			return w, nil
		}
	}
	if len(c.Workspaces) == 0 {
		return Workspace{}, fmt.Errorf("no workspace named %q, and none is configured", name)
	}
	return Workspace{}, fmt.Errorf("no workspace named %q: the configured ones are %s",
		name, strings.Join(c.workspaceNames(), ", "))
}

// MachineFor resolves a workspace to the machine it lives on.
func (c Config) MachineFor(workspace string) (Machine, Workspace, error) {
	w, err := c.Workspace(workspace)
	if err != nil {
		return Machine{}, Workspace{}, err
	}
	m, err := c.Machine(w.Machine)
	if err != nil {
		return Machine{}, w, fmt.Errorf("workspace %q: %w", workspace, err)
	}
	return m, w, nil
}

// WorkspacesOn returns the workspaces that live on a machine, in order.
//
// A workspace with no machine belongs to the only one there is; Validate has
// already refused that shape when there are several.
func (c Config) WorkspacesOn(machine string) []Workspace {
	var out []Workspace
	for _, w := range c.Workspaces {
		if w.Machine == machine || (w.Machine == "" && len(c.Machines) == 1) {
			out = append(out, w)
		}
	}
	return out
}

// Validate reports the first thing that makes the configuration unusable, in
// words that say what to change.
func (c Config) Validate() error {
	if len(c.Machines) == 0 {
		return errors.New("no machine configured: add one under `machines` in " + FileName)
	}

	seen := map[string]bool{}
	for i, m := range c.Machines {
		switch {
		case m.Name == "":
			return fmt.Errorf("machine %d has no name: every entry under `machines` needs a `name`", i+1)
		case seen[m.Name]:
			return fmt.Errorf("two machines are named %q: a machine's name has to be unique", m.Name)
		case len(m.Hosts) == 0:
			return fmt.Errorf("machine %q has no address: add at least one entry under its `hosts`", m.Name)
		case m.Port < 1 || m.Port > 65535:
			return fmt.Errorf("machine %q has port %d: use a port between 1 and 65535", m.Name, m.Port)
		}
		for j, h := range m.Hosts {
			if h.Address == "" {
				return fmt.Errorf("machine %q: host %d is empty", m.Name, j+1)
			}
		}
		seen[m.Name] = true
	}

	names := map[string]bool{}
	for i, w := range c.Workspaces {
		switch {
		case w.Name == "":
			return fmt.Errorf("workspace %d has no name: every entry under `workspaces` needs a `name`", i+1)
		case names[w.Name]:
			return fmt.Errorf("two workspaces are named %q: a workspace's name has to be unique", w.Name)
		case w.Machine == "" && len(c.Machines) > 1:
			return fmt.Errorf(
				"workspace %q does not say which machine it runs on, and there are several (%s): add `machine:` to it",
				w.Name, strings.Join(c.machineNames(), ", "))
		case w.Machine != "" && !seen[w.Machine]:
			return fmt.Errorf("workspace %q runs on machine %q, which is not configured", w.Name, w.Machine)
		}
		names[w.Name] = true
	}
	return nil
}

func (c Config) machineNames() []string {
	out := make([]string, 0, len(c.Machines))
	for _, m := range c.Machines {
		out = append(out, m.Name)
	}
	return out
}

func (c Config) workspaceNames() []string {
	out := make([]string, 0, len(c.Workspaces))
	for _, w := range c.Workspaces {
		out = append(out, w.Name)
	}
	return out
}
