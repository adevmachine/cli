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

func TestMachinesListShowsEachMachineAndItsWorkspaces(t *testing.T) {
	dir := writeConfigDir(t, `
machines:
  - name: main
    hosts: [203.0.113.10]
  - name: sandbox
    hosts: [198.51.100.7]
    port: 2222
workspaces:
  - name: alice
    machine: main
  - name: bob
    machine: sandbox
`)

	out, err := execute(t, "--config", dir, "machines", "list")
	if err != nil {
		t.Fatalf("machines list returned %v", err)
	}
	for _, want := range []string{"main", "sandbox", "alice", "bob", "2222"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output is missing %q: %q", want, out)
		}
	}
}

func TestMachinesListSaysWhenAMachineHasNoWorkspace(t *testing.T) {
	dir := writeConfigDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	out, err := execute(t, "--config", dir, "machines", "list")
	if err != nil {
		t.Fatalf("machines list returned %v", err)
	}
	if !strings.Contains(out, "none") {
		t.Fatalf("an empty machine should say so: %q", out)
	}
}

func TestMachinesListAsJSONKeepsTheWorkspaceMapping(t *testing.T) {
	dir := writeConfigDir(t, `
machines:
  - name: main
    hosts: [203.0.113.10]
  - name: sandbox
    hosts: [198.51.100.7]
workspaces:
  - name: bob
    machine: sandbox
`)

	out, err := execute(t, "--config", dir, "--format", "json", "machines", "list")
	if err != nil {
		t.Fatalf("machines list returned %v", err)
	}
	var got []machineJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output was not JSON: %v (%q)", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("got %d machines", len(got))
	}
	for _, m := range got {
		switch m.Name {
		case "main":
			if len(m.Workspaces) != 0 {
				t.Fatalf("main should have no workspace: %#v", m)
			}
		case "sandbox":
			if len(m.Workspaces) != 1 || m.Workspaces[0] != "bob" {
				t.Fatalf("sandbox should hold bob: %#v", m)
			}
		}
	}
}

func TestSecretsSetThenListShowsTheNameNotTheValue(t *testing.T) {
	dir := t.TempDir()

	if _, err := execute(t, "--config", dir, "secrets", "set", "cloudflare_token", "s3cret"); err != nil {
		t.Fatalf("secrets set returned %v", err)
	}

	out, err := execute(t, "--config", dir, "secrets", "list")
	if err != nil {
		t.Fatalf("secrets list returned %v", err)
	}
	if !strings.Contains(out, "cloudflare_token") {
		t.Fatalf("the name is missing: %q", out)
	}
	if strings.Contains(out, "s3cret") {
		t.Fatal("secrets list printed a value")
	}
}

func TestSecretsSetRefusesAnEmptyValue(t *testing.T) {
	dir := t.TempDir()

	if _, err := execute(t, "--config", dir, "secrets", "set", "token", ""); err == nil {
		t.Fatal("expected an error for an empty value")
	}
}

func TestSecretsRmRemovesIt(t *testing.T) {
	dir := t.TempDir()

	execute(t, "--config", dir, "secrets", "set", "token", "s3cret")
	if _, err := execute(t, "--config", dir, "secrets", "rm", "token"); err != nil {
		t.Fatalf("secrets rm returned %v", err)
	}

	out, _ := execute(t, "--config", dir, "secrets", "list")
	if strings.Contains(out, "token") {
		t.Fatalf("the secret survived rm: %q", out)
	}
}

func TestSecretsRmSaysWhenThereWasNothingToRemove(t *testing.T) {
	if _, err := execute(t, "--config", t.TempDir(), "secrets", "rm", "absent"); err == nil {
		t.Fatal("expected an error for a secret that does not exist")
	}
}

func TestSecretsListAsJSONCarriesOnlyNames(t *testing.T) {
	dir := t.TempDir()
	execute(t, "--config", dir, "secrets", "set", "token", "s3cret")

	out, err := execute(t, "--config", dir, "--format", "json", "secrets", "list")
	if err != nil {
		t.Fatalf("secrets list returned %v", err)
	}
	var got struct {
		Secrets []string `json:"secrets"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output was not JSON: %v (%q)", err, out)
	}
	if len(got.Secrets) != 1 || got.Secrets[0] != "token" {
		t.Fatalf("got %#v", got.Secrets)
	}
	if strings.Contains(out, "s3cret") {
		t.Fatal("the JSON carried a value")
	}
}

func TestSecretsSetReadsTheValueFromStandardInput(t *testing.T) {
	dir := t.TempDir()

	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetIn(strings.NewReader("from-stdin\n"))
	cmd.SetArgs([]string{"--config", dir, "secrets", "set", "token", "--stdin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("secrets set --stdin returned %v", err)
	}

	listed, _ := execute(t, "--config", dir, "secrets", "list")
	if !strings.Contains(listed, "token") {
		t.Fatalf("the secret was not stored: %q", listed)
	}
}

func TestDNSStatusNeedsAHostOrAConfiguredDomain(t *testing.T) {
	dir := writeConfigDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	_, err := execute(t, "--config", dir, "dns", "status")
	if err == nil {
		t.Fatal("expected an error with no host and no domain")
	}
	if !strings.Contains(err.Error(), "domain") {
		t.Fatalf("the error does not say what is missing: %v", err)
	}
}

func TestDNSStatusReportsANameThatDoesNotResolve(t *testing.T) {
	// .invalid never resolves, by standard. No network is needed for that.
	out, err := execute(t, "dns", "status", "nothing.invalid")
	if err == nil {
		t.Fatal("expected an error for a name that does not serve")
	}
	if !strings.Contains(out, "fail") || !strings.Contains(out, "dns") {
		t.Fatalf("the report does not say DNS failed: %q", out)
	}
	if !strings.Contains(out, "skip") {
		t.Fatalf("the later steps should be skipped, not failed: %q", out)
	}
}

func TestHelpJSONListsEveryCommand(t *testing.T) {
	out, err := execute(t, "help", "--json")
	if err != nil {
		t.Fatalf("help --json returned %v", err)
	}

	var got surfaceJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output was not JSON: %v (%q)", err, out)
	}

	names := map[string]bool{}
	for _, c := range got.Commands {
		names[c.Name] = true
	}
	for _, want := range []string{"config", "doctor", "stats", "dns", "secrets", "setup", "machines", "ssh", "mosh", "run"} {
		if !names[want] {
			t.Fatalf("%q is missing from help --json", want)
		}
	}
}

func TestHelpJSONCarriesSubcommandsAndFlags(t *testing.T) {
	out, _ := execute(t, "help", "--json")

	var got surfaceJSON
	json.Unmarshal([]byte(out), &got)

	for _, c := range got.Commands {
		if c.Name != "config" {
			continue
		}
		sub := map[string]bool{}
		for _, s := range c.Sub {
			sub[s.Name] = true
		}
		if !sub["path"] || !sub["show"] {
			t.Fatalf("config's subcommands are missing: %#v", c.Sub)
		}
		return
	}
	t.Fatal("config is not in the surface")
}

func TestHelpJSONDoesNotOfferCobrasCompletionCommand(t *testing.T) {
	out, _ := execute(t, "help", "--json")

	var got surfaceJSON
	json.Unmarshal([]byte(out), &got)

	for _, c := range got.Commands {
		if c.Name == "completion" {
			t.Fatal("completion is cobra's, not part of what this CLI offers")
		}
	}
}

func TestHelpJSONCarriesTheGlobalFlags(t *testing.T) {
	out, _ := execute(t, "help", "--json")

	var got surfaceJSON
	json.Unmarshal([]byte(out), &got)

	names := map[string]bool{}
	for _, f := range got.Flags {
		names[f.Name] = true
	}
	for _, want := range []string{"config", "format", "machine"} {
		if !names[want] {
			t.Fatalf("the global flag %q is missing: %#v", want, got.Flags)
		}
	}
}

func TestTheSurfaceListsEveryCommandPath(t *testing.T) {
	out := &bytes.Buffer{}
	if err := WriteSurface(out); err != nil {
		t.Fatalf("WriteSurface returned %v", err)
	}
	for _, want := range []string{"devmachine", "devmachine config path", "devmachine machines list", "devmachine secrets set"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("the surface is missing %q:\n%s", want, out)
		}
	}
}

func TestMachinesListInJSONSaysWhichMachineIsThisComputer(t *testing.T) {
	dir := configWith(t, `
machines:
  - name: main
    hosts: [203.0.113.10]
  - name: mac
    self: true
`)
	out, err := execute(t, "--config", dir, "--format", "json", "machines", "list")
	if err != nil {
		t.Fatal(err)
	}
	var got []machineJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err, out)
	}
	for _, m := range got {
		if m.Self != (m.Name == "mac") {
			t.Fatalf("%s: self=%v", m.Name, m.Self)
		}
	}
}
