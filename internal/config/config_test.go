package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirPrefersTheFlagOverEverything(t *testing.T) {
	t.Setenv("DEVMACHINE_CONFIG", "/from/env")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	dir, source, err := Dir("/from/flag")
	if err != nil {
		t.Fatalf("Dir returned %v", err)
	}
	if dir != "/from/flag" || source != SourceFlag {
		t.Fatalf("got (%q, %q), want (%q, %q)", dir, source, "/from/flag", SourceFlag)
	}
}

func TestDirFallsBackToTheEnvironment(t *testing.T) {
	t.Setenv("DEVMACHINE_CONFIG", "/from/env")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	dir, source, err := Dir("")
	if err != nil {
		t.Fatalf("Dir returned %v", err)
	}
	if dir != "/from/env" || source != SourceEnv {
		t.Fatalf("got (%q, %q), want (%q, %q)", dir, source, "/from/env", SourceEnv)
	}
}

func TestDirFallsBackToXDG(t *testing.T) {
	t.Setenv("DEVMACHINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	dir, source, err := Dir("")
	if err != nil {
		t.Fatalf("Dir returned %v", err)
	}
	want := filepath.Join("/xdg", "devmachine")
	if dir != want || source != SourceXDG {
		t.Fatalf("got (%q, %q), want (%q, %q)", dir, source, want, SourceXDG)
	}
}

func TestDirFallsBackToTheHomeDirectory(t *testing.T) {
	t.Setenv("DEVMACHINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/alice")

	dir, source, err := Dir("")
	if err != nil {
		t.Fatalf("Dir returned %v", err)
	}
	want := filepath.Join("/home/alice", ".config", "devmachine")
	if dir != want || source != SourceDefault {
		t.Fatalf("got (%q, %q), want (%q, %q)", dir, source, want, SourceDefault)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadReadsHostsInOrder(t *testing.T) {
	dir := writeConfig(t, "hosts:\n  - tailscale:vps\n  - 203.0.113.10\ndomain: example.com\n")

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	if len(c.Hosts) != 2 {
		t.Fatalf("got %d hosts, want 2: %#v", len(c.Hosts), c.Hosts)
	}
	if c.Hosts[0].Address != "tailscale:vps" || c.Hosts[1].Address != "203.0.113.10" {
		t.Fatalf("hosts came back in the wrong order: %#v", c.Hosts)
	}
	if c.Domain != "example.com" {
		t.Fatalf("domain = %q", c.Domain)
	}
}

func TestLoadDefaultsTheUserAndThePort(t *testing.T) {
	dir := writeConfig(t, "hosts:\n  - 203.0.113.10\n")

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	if c.User != "root" {
		t.Fatalf("user = %q, want \"root\"", c.User)
	}
	if c.Port != 22 {
		t.Fatalf("port = %d, want 22", c.Port)
	}
}

func TestLoadKeepsAnExplicitUserAndPort(t *testing.T) {
	dir := writeConfig(t, "hosts:\n  - 203.0.113.10\nuser: alice\nport: 2222\n")

	c, _ := Load(dir)
	if c.User != "alice" || c.Port != 2222 {
		t.Fatalf("got user %q port %d", c.User, c.Port)
	}
}

func TestLoadSaysWhichFileIsMissing(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected an error when config.yml is missing")
	}
	if !strings.Contains(err.Error(), FileName) {
		t.Fatalf("the error does not name the file: %v", err)
	}
}

func TestLoadRejectsBrokenYAML(t *testing.T) {
	dir := writeConfig(t, "hosts: [unclosed\n")

	if _, err := Load(dir); err == nil {
		t.Fatal("expected an error for broken YAML")
	}
}

func TestValidateRejectsAConfigWithNoHost(t *testing.T) {
	err := Config{Domain: "example.com", User: "root", Port: 22}.Validate()
	if err == nil {
		t.Fatal("expected an error when there is no host")
	}
	if !strings.Contains(err.Error(), "hosts") {
		t.Fatalf("the error does not say what to fix: %v", err)
	}
}

func TestValidateRejectsAnEmptyHostAddress(t *testing.T) {
	c := Config{Hosts: []Host{{Address: ""}}, User: "root", Port: 22}
	if err := c.Validate(); err == nil {
		t.Fatal("expected an error for an empty host address")
	}
}

func TestValidateRejectsAPortOutOfRange(t *testing.T) {
	for _, port := range []int{0, -1, 70000} {
		c := Config{Hosts: []Host{{Address: "203.0.113.10"}}, User: "root", Port: port}
		if err := c.Validate(); err == nil {
			t.Fatalf("expected an error for port %d", port)
		}
	}
}

func TestValidateAcceptsAMinimalConfig(t *testing.T) {
	c := Config{Hosts: []Host{{Address: "203.0.113.10"}}, User: "root", Port: 22}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate returned %v", err)
	}
}

func TestLoadReadsThePrivateKeyPath(t *testing.T) {
	dir := writeConfig(t, "hosts:\n  - 203.0.113.10\nkey: /home/alice/.ssh/id_ed25519\n")

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	if c.Key != "/home/alice/.ssh/id_ed25519" {
		t.Fatalf("key = %q", c.Key)
	}
}

func TestLoadLeavesTheKeyEmptyWhenTheAgentIsMeantToServe(t *testing.T) {
	dir := writeConfig(t, "hosts:\n  - 203.0.113.10\n")

	c, _ := Load(dir)
	if c.Key != "" {
		t.Fatalf("key = %q, want empty so the agent is used", c.Key)
	}
}
