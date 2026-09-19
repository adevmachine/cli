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
// property of the workspace, so `devmachine ssh alice` needs no address and no
// machine: the mapping already says which server that is.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// MarshalYAML writes a host back as the bare string it was read from.
func (h Host) MarshalYAML() (any, error) { return h.Address, nil }

// Machine is a server the CLI operates.
type Machine struct {
	Name string `yaml:"name"`
	// Hosts are tried in order, so the first is the preferred path and the
	// rest are fallbacks.
	Hosts []Host `yaml:"hosts"`
	// User is the administrative login used to provision, not a workspace.
	User string `yaml:"user,omitempty"`
	Port int    `yaml:"port,omitempty"`
	// Key is a private key on disk. Left empty, the SSH agent serves the keys
	// instead, which is how a 1Password-style agent is supported without this
	// package knowing such a thing exists.
	Key string `yaml:"key,omitempty"`
	// Packages are the recipes this machine gets, by name.
	Packages []string `yaml:"packages,omitempty"`
}

// Workspace is an environment: one Linux user on one machine.
type Workspace struct {
	Name string `yaml:"name"`
	// Machine names where this workspace lives. It may be left out when the
	// configuration has exactly one machine.
	Machine string `yaml:"machine,omitempty"`
	// User overrides the Linux account. It is only needed when the workspace
	// name cannot be the account name.
	User string `yaml:"user,omitempty"`
	// Packages are the recipes this workspace gets, by name.
	Packages []string `yaml:"packages,omitempty"`
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
	Workspaces  []Workspace `yaml:"workspaces,omitempty"`
	Domain      string      `yaml:"domain,omitempty"`
	DNSProvider string      `yaml:"dns_provider,omitempty"`
	// Packages is the release of the packages repository every recipe is read
	// from, for example "v1".
	Packages string `yaml:"packages,omitempty"`
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
		if name, dup := firstDuplicate(m.Packages); dup {
			return fmt.Errorf("machine %q lists the package %q twice", m.Name, name)
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
		if name, dup := firstDuplicate(w.Packages); dup {
			return fmt.Errorf("workspace %q lists the package %q twice", w.Name, name)
		}
		names[w.Name] = true
	}

	// A pin is a release tag, never a branch: `packages: main` would mean the
	// set changes under you because somebody pushed an hour ago. Upgrading is
	// meant to be a deliberate act with a diff to read.
	if c.Packages != "" && !releaseTag.MatchString(c.Packages) {
		return fmt.Errorf(
			"`packages: %s` is not a release tag: use one like `v1`, never a branch name", c.Packages)
	}
	return nil
}

// releaseTag is deliberately loose about the shape after the v: the packages
// repository decides how it numbers its releases, not this package.
var releaseTag = regexp.MustCompile(`^v[0-9][0-9A-Za-z.\-]*$`)

func firstDuplicate(names []string) (string, bool) {
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			return n, true
		}
		seen[n] = true
	}
	return "", false
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

// Save writes the configuration back to dir.
//
// Only what the CLI edits is written: the package pin, and each target's
// package list. Everything else in the file is left as the person wrote it,
// comments included, because a file that loses its comments the first time the
// CLI touches it is a file people stop letting the CLI touch.
//
// That is also why the struct is not simply marshalled over the top: Load
// fills in the admin user and the port, so a full rewrite would put values in
// the file that nobody chose and that stop tracking the defaults.
func Save(dir string, c Config) error {
	path := filepath.Join(dir, FileName)

	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return writeYAML(path, &c, 0o600)
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return writeYAML(path, &c, mode)
	}

	root := document.Content[0]
	setField(root, "packages", stringNode(c.Packages))
	for _, m := range c.Machines {
		if err := setPackages(root, "machines", m.Name, m.Packages); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	for _, w := range c.Workspaces {
		if err := setPackages(root, "workspaces", w.Name, w.Packages); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	return writeYAML(path, &document, mode)
}

// setPackages puts one target's package list on the entry it belongs to.
func setPackages(root *yaml.Node, section, name string, list []string) error {
	entries := field(root, section)
	if entries != nil && entries.Kind == yaml.SequenceNode {
		for _, entry := range entries.Content {
			if entry.Kind == yaml.MappingNode && scalar(field(entry, "name")) == name {
				setField(entry, "packages", sequenceNode(list))
				return nil
			}
		}
	}
	return fmt.Errorf("`%s` has no entry named %q", section, name)
}

// setField replaces a key's value, appends the key when it is not there, and
// removes it when the value is nil. An empty list or an empty pin is written
// as nothing rather than as `packages: []`.
func setField(node *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != key {
			continue
		}
		if value == nil {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
		value.HeadComment = node.Content[i+1].HeadComment
		value.LineComment = node.Content[i+1].LineComment
		value.FootComment = node.Content[i+1].FootComment
		node.Content[i+1] = value
		return
	}
	if value == nil {
		return
	}
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func field(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func scalar(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	return node.Value
}

func stringNode(value string) *yaml.Node {
	if value == "" {
		return nil
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func sequenceNode(values []string) *yaml.Node {
	if len(values) == 0 {
		return nil
	}
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, v := range values {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
	}
	return node
}

func writeYAML(path string, value any, mode os.FileMode) error {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), mode); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
