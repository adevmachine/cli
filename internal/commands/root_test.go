package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// execute runs the command tree with args and returns what it printed.
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestVersionPrintsTheVersion(t *testing.T) {
	SetVersion("1.2.3")
	t.Cleanup(func() { SetVersion("dev") })

	out, err := execute(t, "version")
	if err != nil {
		t.Fatalf("version returned %v", err)
	}
	if !strings.Contains(out, "1.2.3") {
		t.Fatalf("output did not carry the version: %q", out)
	}
}

func TestUnknownCommandFails(t *testing.T) {
	if _, err := execute(t, "nope"); err == nil {
		t.Fatal("expected an error for an unknown command")
	}
}

func TestUnknownFormatIsRejected(t *testing.T) {
	_, err := execute(t, "--format", "yaml", "version")
	if err == nil {
		t.Fatal("expected an error for an unknown --format")
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Fatalf("the error does not name the bad value: %v", err)
	}
}

func TestConfigPathNamesThePathAndItsSource(t *testing.T) {
	t.Setenv("DEVMACHINE_CONFIG", "/from/env")

	out, err := execute(t, "config", "path")
	if err != nil {
		t.Fatalf("config path returned %v", err)
	}
	if !strings.Contains(out, "/from/env") || !strings.Contains(out, "env") {
		t.Fatalf("output did not name the path and its source: %q", out)
	}
}

func TestConfigPathFlagBeatsTheEnvironment(t *testing.T) {
	t.Setenv("DEVMACHINE_CONFIG", "/from/env")

	out, err := execute(t, "--config", "/from/flag", "config", "path")
	if err != nil {
		t.Fatalf("config path returned %v", err)
	}
	if !strings.Contains(out, "/from/flag") {
		t.Fatalf("the flag did not win: %q", out)
	}
}

func TestConfigPathAsJSON(t *testing.T) {
	t.Setenv("DEVMACHINE_CONFIG", "/from/env")

	out, err := execute(t, "--format", "json", "config", "path")
	if err != nil {
		t.Fatalf("config path returned %v", err)
	}
	var got struct {
		Path   string `json:"path"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output was not JSON: %v (%q)", err, out)
	}
	if got.Path != "/from/env" || got.Source != "env" {
		t.Fatalf("got %#v", got)
	}
}

func writeConfigDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestConfigShowPrintsTheEffectiveValues(t *testing.T) {
	dir := writeConfigDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\ndomain: example.com\n")

	out, err := execute(t, "--config", dir, "config", "show")
	if err != nil {
		t.Fatalf("config show returned %v", err)
	}
	for _, want := range []string{"203.0.113.10", "example.com", "root", "22"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output is missing %q: %q", want, out)
		}
	}
}

func TestConfigShowAsJSONCarriesEveryHost(t *testing.T) {
	dir := writeConfigDir(t, "machines:\n  - name: main\n    hosts: [tailscale:vps, 203.0.113.10]\n")

	out, err := execute(t, "--config", dir, "--format", "json", "config", "show")
	if err != nil {
		t.Fatalf("config show returned %v", err)
	}
	var got configJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output was not JSON: %v (%q)", err, out)
	}
	if len(got.Machines) != 1 {
		t.Fatalf("machines = %#v", got.Machines)
	}
	m := got.Machines[0]
	if len(m.Hosts) != 2 || m.Hosts[0] != "tailscale:vps" {
		t.Fatalf("hosts = %#v", m.Hosts)
	}
	if m.AdminUser != "root" || m.Port != 22 {
		t.Fatalf("defaults were not applied: %#v", m)
	}
}

func TestConfigShowFailsWhenThereIsNoConfig(t *testing.T) {
	if _, err := execute(t, "--config", t.TempDir(), "config", "show"); err == nil {
		t.Fatal("expected an error when config.yml is missing")
	}
}

func TestConfigShowReportsAnInvalidConfig(t *testing.T) {
	dir := writeConfigDir(t, "domain: example.com\n")

	_, err := execute(t, "--config", dir, "config", "show")
	if err == nil {
		t.Fatal("expected an error when there is no machine")
	}
	if !strings.Contains(err.Error(), "machines") {
		t.Fatalf("the error does not say what to fix: %v", err)
	}
}
