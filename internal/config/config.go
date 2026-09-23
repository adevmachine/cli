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
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the configuration file inside the configuration directory.
const FileName = "config.yml"

// KnownHostsFileName is the configuration-scoped SSH host trust store.
const KnownHostsFileName = "known_hosts"

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

// The two answers an operator can give about a credential.
//
// CredentialMachine asks for one login to serve every workspace that says so;
// CredentialOwn makes a workspace log in for itself.
const (
	CredentialMachine = "machine"
	CredentialOwn     = "own"
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
	// Settings override the variables a package declares. A key is written
	// `<package>.<name>`.
	Settings map[string]any `yaml:"settings,omitempty"`
	// KnownHostsFile is runtime metadata resolved from the configuration
	// directory. It is never another source of user configuration.
	KnownHostsFile string `yaml:"-"`
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
	// Settings override the variables a package declares. A key is written
	// `<package>.<name>`.
	Settings map[string]any `yaml:"settings,omitempty"`
	// Credentials is this workspace's own answer, and it overrides the
	// configuration's.
	Credentials map[string]string `yaml:"credentials,omitempty"`
}

// LinuxUser is the account this workspace owns on its machine.
func (w Workspace) LinuxUser() string {
	if w.User != "" {
		return w.User
	}
	return w.Name
}

// DefaultWorkspacePackages is what `setup` seeds `defaults.workspace` with.
//
// It is a seed, not a rule: it is written into the person's own file, where
// they can change it, and nothing reads it again afterwards.
var DefaultWorkspacePackages = []string{"workspace", "dev", "zsh", "mise"}

// Defaults are the choices something new gets when nothing says otherwise.
//
// They live in the configuration rather than in the binary, so changing one
// line changes every workspace made afterwards, and two people with different
// habits do not need two builds.
type Defaults struct {
	// Workspace is the package list a new workspace is created with.
	Workspace []string `yaml:"workspace,omitempty"`
}

// Config is what config.yml holds.
type Config struct {
	Machines    []Machine   `yaml:"machines"`
	Workspaces  []Workspace `yaml:"workspaces,omitempty"`
	Defaults    Defaults    `yaml:"defaults,omitempty"`
	Domain      string      `yaml:"domain,omitempty"`
	DNSProvider string      `yaml:"dns_provider,omitempty"`
	// Packages is the release of the packages repository every recipe is read
	// from, for example "v1".
	Packages string `yaml:"packages,omitempty"`
	// Credentials is what the operator wants done about each credential:
	// CredentialMachine or CredentialOwn, by credential name.
	//
	// It holds no values and no method. The package still says how a
	// credential is obtained and whether a copy of it works anywhere else;
	// this says only whether the operator wants it copied. The first is a fact
	// about a tool, the second is a preference, and preferences belong to
	// whoever has them.
	Credentials map[string]string `yaml:"credentials,omitempty"`
}

// CredentialsFor is the operator's answer about each credential for one
// workspace: the workspace's own preference over the configuration's.
//
// The result is a fresh map, so reading one workspace's answer never edits
// anybody else's.
func (c Config) CredentialsFor(w Workspace) map[string]string {
	out := make(map[string]string, len(c.Credentials)+len(w.Credentials))
	maps.Copy(out, c.Credentials)
	maps.Copy(out, w.Credentials)
	return out
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
		c.Machines[i].KnownHostsFile = filepath.Join(dir, KnownHostsFileName)
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
		case !machineIdentityName.MatchString(m.Name):
			return fmt.Errorf("machine %q does not have a safe SSH identity name: use only letters, numbers, dots, underscores and hyphens, starting with a letter or number", m.Name)
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
		if err := validateSettings("machine", m.Name, m.Settings, m.Packages); err != nil {
			return err
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
		if err := validateSettings("workspace", w.Name, w.Settings, w.Packages); err != nil {
			return err
		}
		if err := validateCredentials("workspace", w.Name, w.Credentials); err != nil {
			return err
		}
		names[w.Name] = true
	}

	if err := validateCredentials("the configuration", "", c.Credentials); err != nil {
		return err
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

// SettingsFor returns the settings for one package on one target, with the
// "<package>." prefix stripped.
//
// Only the first dot segment is the package name, so a package is free to use
// a dotted key of its own.
func SettingsFor(settings map[string]any, pkg string) map[string]any {
	out := map[string]any{}
	for key, value := range settings {
		name, rest, ok := strings.Cut(key, ".")
		if ok && name == pkg && rest != "" {
			out[rest] = value
		}
	}
	return out
}

// validateSettings refuses a setting that would reach nothing.
//
// A value for a package the target does not install is worse than an error:
// the recipe quietly keeps its default, and the machine is not what the
// configuration says it is. A mistyped package name is exactly that.
func validateSettings(kind, target string, settings map[string]any, installed []string) error {
	has := map[string]bool{}
	for _, name := range installed {
		has[name] = true
	}
	for _, key := range slices.Sorted(maps.Keys(settings)) {
		pkg, name, ok := strings.Cut(key, ".")
		if !ok || pkg == "" || name == "" {
			return fmt.Errorf(
				"%s %q: the setting %q does not say which package it belongs to: write it as <package>.<name>",
				kind, target, key)
		}
		if !has[pkg] {
			return fmt.Errorf(
				"%s %q sets %q, and does not install the package %q: add %q to its `packages:`, or drop the setting",
				kind, target, key, pkg, pkg)
		}
	}
	return nil
}

// validateCredentials refuses an answer nothing knows what to do with.
//
// There are two answers and no third, so a typo is caught here rather than
// read as "leave it alone" and silently ignored.
func validateCredentials(kind, target string, preferences map[string]string) error {
	where := kind
	if target != "" {
		where = fmt.Sprintf("%s %q", kind, target)
	}
	for _, name := range slices.Sorted(maps.Keys(preferences)) {
		if value := preferences[name]; value != CredentialMachine && value != CredentialOwn {
			return fmt.Errorf(
				"%s says `%s: %s`: the answers are %q, for one login shared across the workspaces "+
					"that want it, and %q, for a workspace that logs in by itself",
				where, name, value, CredentialMachine, CredentialOwn)
		}
	}
	return nil
}

// releaseTag is deliberately loose about the shape after the v: the packages
// repository decides how it numbers its releases, not this package.
var releaseTag = regexp.MustCompile(`^v[0-9][0-9A-Za-z.\-]*$`)

var machineIdentityName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

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

	mode := modeOf(path)

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

// AddMachine appends a machine to config.yml.
//
// Like Save, it edits the document rather than marshalling over the top, so
// everything the person wrote — comments included — is still there afterwards.
func AddMachine(dir string, m Machine) error {
	path := filepath.Join(dir, FileName)

	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("there is no %s to add to: run `devmachine setup` for the first machine", path)
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s is not a configuration: run `devmachine setup` for the first machine", path)
	}

	root := document.Content[0]
	machines := field(root, "machines")
	if machines == nil || machines.Kind != yaml.SequenceNode {
		machines = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		setField(root, "machines", machines)
	}
	for _, entry := range machines.Content {
		if scalar(field(entry, "name")) == m.Name {
			return fmt.Errorf("a machine named %q is already configured: pick another name", m.Name)
		}
	}
	machines.Content = append(machines.Content, machineNode(m))

	return writeYAML(path, &document, modeOf(path))
}

// RemoveMachine takes a machine out of config.yml.
//
// It removes the entry and nothing else. The server it named keeps running:
// forgetting a machine is not destroying one, and this package could not
// destroy anything if it tried.
func RemoveMachine(dir, name string) error {
	path := filepath.Join(dir, FileName)

	current, err := Load(dir)
	if err != nil {
		return err
	}
	if _, err := current.Machine(name); err != nil {
		return err
	}
	if orphans := current.workspacesLosing(name); len(orphans) > 0 {
		return fmt.Errorf(
			"machine %q still holds the workspace %s: move %s to another machine, or remove it, first",
			name, strings.Join(orphans, ", "), plural(len(orphans), "it", "them"))
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s is not a configuration", path)
	}

	machines := field(document.Content[0], "machines")
	if machines == nil || machines.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s has no `machines`", path)
	}
	for i, entry := range machines.Content {
		if scalar(field(entry, "name")) != name {
			continue
		}
		machines.Content = append(machines.Content[:i], machines.Content[i+1:]...)
		return writeYAML(path, &document, modeOf(path))
	}
	return fmt.Errorf("no machine named %q in %s", name, path)
}

// workspacesLosing names the workspaces that would be left pointing at nothing
// if that machine went away.
func (c Config) workspacesLosing(machine string) []string {
	var out []string
	for _, w := range c.Workspaces {
		if w.Machine == machine || (w.Machine == "" && len(c.Machines) == 1) {
			out = append(out, w.Name)
		}
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// machineNode is a machine as the file holds it.
func machineNode(m Machine) *yaml.Node {
	addresses := make([]string, 0, len(m.Hosts))
	for _, h := range m.Hosts {
		addresses = append(addresses, h.Address)
	}

	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setField(node, "name", stringNode(m.Name))
	setField(node, "hosts", sequenceNode(addresses))
	setField(node, "user", stringNode(m.User))
	setField(node, "port", intNode(m.Port))
	setField(node, "key", stringNode(m.Key))
	return node
}

func intNode(value int) *yaml.Node {
	if value == 0 {
		return nil
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(value)}
}

// modeOf keeps a file's own permissions when it is rewritten.
func modeOf(path string) os.FileMode {
	if info, err := os.Stat(path); err == nil {
		return info.Mode().Perm()
	}
	return 0o600
}

// AddWorkspace appends a workspace to config.yml.
//
// Like AddMachine, it edits the document rather than marshalling over the top,
// so everything the person wrote — comments included — is still there
// afterwards.
func AddWorkspace(dir string, w Workspace) error {
	return editDocument(dir, func(root *yaml.Node) error {
		workspaces := field(root, "workspaces")
		if workspaces == nil || workspaces.Kind != yaml.SequenceNode {
			workspaces = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			setField(root, "workspaces", workspaces)
		}
		for _, entry := range workspaces.Content {
			if scalar(field(entry, "name")) == w.Name {
				return fmt.Errorf("a workspace named %q is already configured: pick another name", w.Name)
			}
		}
		node, err := workspaceNode(w)
		if err != nil {
			return err
		}
		workspaces.Content = append(workspaces.Content, node)
		return nil
	})
}

// UpdateWorkspace writes one workspace's fields back onto its entry.
//
// Only what the CLI edits is written: the machine, the account, the package
// list and the settings. A field left empty is removed rather than written as
// an empty value, so a workspace that no longer overrides its account reads
// like one that never did.
func UpdateWorkspace(dir string, w Workspace) error {
	return editDocument(dir, func(root *yaml.Node) error {
		workspaces := field(root, "workspaces")
		if workspaces != nil && workspaces.Kind == yaml.SequenceNode {
			for _, entry := range workspaces.Content {
				if entry.Kind != yaml.MappingNode || scalar(field(entry, "name")) != w.Name {
					continue
				}
				settings, err := settingsNode(w.Settings)
				if err != nil {
					return err
				}
				setField(entry, "machine", stringNode(w.Machine))
				setField(entry, "user", stringNode(w.User))
				setField(entry, "packages", sequenceNode(w.Packages))
				setField(entry, "settings", settings)
				setField(entry, "credentials", credentialsNode(w.Credentials))
				return nil
			}
		}
		return fmt.Errorf("`workspaces` has no entry named %q", w.Name)
	})
}

// RemoveWorkspace takes a workspace out of config.yml.
//
// It removes the entry and nothing else. The Linux account, its home and its
// files stay on the machine: a configuration edit is not a licence to delete
// somebody's work, and `sync` could not put it back.
func RemoveWorkspace(dir, name string) error {
	current, err := Load(dir)
	if err != nil {
		return err
	}
	if _, err := current.Workspace(name); err != nil {
		return err
	}

	return editDocument(dir, func(root *yaml.Node) error {
		workspaces := field(root, "workspaces")
		if workspaces == nil || workspaces.Kind != yaml.SequenceNode {
			return fmt.Errorf("`workspaces` has no entry named %q", name)
		}
		for i, entry := range workspaces.Content {
			if scalar(field(entry, "name")) != name {
				continue
			}
			workspaces.Content = append(workspaces.Content[:i], workspaces.Content[i+1:]...)
			if len(workspaces.Content) == 0 {
				setField(root, "workspaces", nil)
			}
			return nil
		}
		return fmt.Errorf("`workspaces` has no entry named %q", name)
	})
}

// editDocument reads config.yml as a node tree, hands the root to edit, and
// writes it back with its permissions and its comments intact.
func editDocument(dir string, edit func(root *yaml.Node) error) error {
	path := filepath.Join(dir, FileName)

	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("there is no %s to edit: run `devmachine setup` for the first machine", path)
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s is not a configuration: run `devmachine setup` for the first machine", path)
	}

	if err := edit(document.Content[0]); err != nil {
		return err
	}
	return writeYAML(path, &document, modeOf(path))
}

// workspaceNode is a workspace as the file holds it.
func workspaceNode(w Workspace) (*yaml.Node, error) {
	settings, err := settingsNode(w.Settings)
	if err != nil {
		return nil, err
	}

	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setField(node, "name", stringNode(w.Name))
	setField(node, "machine", stringNode(w.Machine))
	setField(node, "user", stringNode(w.User))
	setField(node, "packages", sequenceNode(w.Packages))
	setField(node, "settings", settings)
	return node, nil
}

// settingsNode renders a settings map with its keys in order, so two runs that
// set the same things produce the same file.
// credentialsNode renders a workspace's sharing choices.
//
// It is separate from settingsNode because a choice is always a string and can
// never fail to encode, so the caller has no error to handle.
func credentialsNode(choices map[string]string) *yaml.Node {
	if len(choices) == 0 {
		return nil
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, name := range slices.Sorted(maps.Keys(choices)) {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: choices[name]})
	}
	return node
}

func settingsNode(settings map[string]any) (*yaml.Node, error) {
	if len(settings) == 0 {
		return nil, nil
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, key := range slices.Sorted(maps.Keys(settings)) {
		value := &yaml.Node{}
		if err := value.Encode(settings[key]); err != nil {
			return nil, fmt.Errorf("writing the setting %q: %w", key, err)
		}
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
	}
	return node, nil
}
