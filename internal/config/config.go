// Package config resolves, reads and validates the user's configuration.
//
// The configuration is the user's, not the CLI's: it lives in a directory they
// choose and this repository never holds a copy of it.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileName is the configuration file inside the configuration directory.
const FileName = "config.yml"

// Source says which rule chose the configuration directory.
type Source string

// The rules, in the order they are tried.
const (
	SourceFlag    Source = "flag"
	SourceEnv     Source = "env"
	SourceXDG     Source = "xdg"
	SourceDefault Source = "default"
)

// EnvVar overrides the configuration directory.
const EnvVar = "DEVMACHINE_CONFIG"

// Host is one address the machine answers on.
//
// An address is either a literal (an IP or a name that DNS resolves) or a
// `tailscale:<machine>` entry, which something closer to the network resolves.
// Parsing an address is not this package's job; keeping their order is.
type Host struct {
	Address string
}

// UnmarshalYAML lets a host be written as a bare string in config.yml.
func (h *Host) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	h.Address = s
	return nil
}

// Config is what config.yml holds.
type Config struct {
	// Hosts are tried in order, so the first is the preferred path and the
	// rest are fallbacks.
	Hosts       []Host `yaml:"hosts"`
	Domain      string `yaml:"domain"`
	User        string `yaml:"user"`
	Port        int    `yaml:"port"`
	DNSProvider string `yaml:"dns_provider"`
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

// Load reads and fills in config.yml from dir.
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

	if c.User == "" {
		c.User = "root"
	}
	if c.Port == 0 {
		c.Port = 22
	}
	return c, nil
}

// Validate reports the first thing that makes the configuration unusable, in
// words that say what to change.
func (c Config) Validate() error {
	if len(c.Hosts) == 0 {
		return errors.New("no host configured: add at least one address under `hosts` in " + FileName)
	}
	for i, h := range c.Hosts {
		if h.Address == "" {
			return fmt.Errorf("host %d is empty: every entry under `hosts` needs an address", i+1)
		}
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port %d is out of range: use a port between 1 and 65535", c.Port)
	}
	return nil
}
