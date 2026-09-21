package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
)

func writeBrokenPackage(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "git")
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.yml"), []byte("format: 1\nname: git\nscope: user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "---\n- name: Install git\n  apt:\n    name: git\n"
	if err := os.WriteFile(filepath.Join(dir, "tasks", "main.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPackagesValidatePrintsEveryProblemAtOnce(t *testing.T) {
	dir := writeBrokenPackage(t)

	out, err := execute(t, "packages", "validate", dir)
	if err == nil {
		t.Fatal("a broken package passed")
	}
	for _, want := range []string{"scope", "summary", "apt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the output leaves out %q: %s", want, out)
		}
	}
}

func TestPackagesValidateAsJSONCarriesEachProblemWithItsPlace(t *testing.T) {
	dir := writeBrokenPackage(t)

	out, _ := execute(t, "--format", "json", "packages", "validate", dir)
	var got struct {
		OK       bool `json:"ok"`
		Problems []struct {
			File string `json:"file"`
			Line int    `json:"line"`
			What string `json:"what"`
		} `json:"problems"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, out)
	}
	if got.OK || len(got.Problems) == 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestPackagesValidateAcceptsAGoodPackage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sharing")
	if err := packages.WriteSkeleton(dir, "sharing", packages.ScopeWorkspace, ""); err != nil {
		t.Fatal(err)
	}

	if _, err := execute(t, "packages", "validate", dir); err != nil {
		t.Fatalf("the skeleton did not pass: %v", err)
	}
}

func TestPackagesNewWritesSomethingValidateAccepts(t *testing.T) {
	into := t.TempDir()

	if _, err := execute(t, "packages", "new", "sharing", "--scope", "workspace", "--into", into); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, "packages", "validate", filepath.Join(into, "sharing")); err != nil {
		t.Fatalf("what `packages new` wrote does not validate: %v", err)
	}
}

func TestPackagesNewRefusesToOverwrite(t *testing.T) {
	into := t.TempDir()

	if _, err := execute(t, "packages", "new", "sharing", "--scope", "workspace", "--into", into); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, "packages", "new", "sharing", "--scope", "workspace", "--into", into); err == nil {
		t.Fatal("it overwrote a package that was already there")
	}
}

func TestPackagesSchemaJSONNamesEveryField(t *testing.T) {
	out, err := execute(t, "packages", "schema", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Formats []int `json:"formats"`
		Fields  []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
			Summary  string `json:"summary"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	for _, want := range []string{"format", "name", "scope", "summary", "needs", "provides", "extends", "credentials"} {
		found := false
		for _, f := range got.Fields {
			if f.Name == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("the schema leaves out %q", want)
		}
	}
	// Whoever writes a recipe has to know which format number to put at the
	// top, and asking the binary is the only answer that cannot drift.
	if !slices.Equal(got.Formats, packages.ReadableFormats) {
		t.Fatalf("the schema does not publish the formats it reads: %#v", got.Formats)
	}
}

func TestPackagesSchemaAsATablePrintsTheFormatAndTheFields(t *testing.T) {
	out, err := execute(t, "packages", "schema")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"format", "scope", "required"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the table leaves out %q: %s", want, out)
		}
	}
}

func configWithMachineAndWorkspace(t *testing.T) string {
	t.Helper()
	return configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\nworkspaces:\n  - name: alice\n")
}

// writeLocalPackage puts a package in the operator's own directory, which is
// where a configuration with no release pin finds everything it has.
func writeLocalPackage(t *testing.T, configDir, name, scope string) {
	t.Helper()
	if err := packages.WriteSkeleton(filepath.Join(packages.LocalDir(configDir), name), name, scope, ""); err != nil {
		t.Fatal(err)
	}
}

func configWithPackagesInstalled(t *testing.T) string {
	t.Helper()
	dir := configWith(t, `
machines:
  - name: main
    hosts: [203.0.113.10]
    packages: [docker]
workspaces:
  - name: alice
    packages: [claude-code]
`)
	writeLocalPackage(t, dir, "docker", packages.ScopeMachine)
	writeLocalPackage(t, dir, "claude-code", packages.ScopeWorkspace)
	writeLocalPackage(t, dir, "caddy", packages.ScopeMachine)
	return dir
}

func TestPackagesAddPutsItOnTheNamedWorkspace(t *testing.T) {
	dir := configWithMachineAndWorkspace(t)

	if _, err := execute(t, "--config", dir, "packages", "add", "claude-code", "--workspace", "alice", "--yes"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workspaces[0].Packages) != 1 || cfg.Workspaces[0].Packages[0] != "claude-code" {
		t.Fatalf("got %#v", cfg.Workspaces[0].Packages)
	}
	if len(cfg.Machines[0].Packages) != 0 {
		t.Fatalf("it also touched the machine: %#v", cfg.Machines[0].Packages)
	}
}

func TestPackagesAddRefusesTwoTargets(t *testing.T) {
	dir := configWithMachineAndWorkspace(t)

	_, err := execute(t, "--config", dir, "packages", "add", "docker", "--workspace", "alice", "--machine", "main", "--yes")
	if err == nil {
		t.Fatal("two targets were accepted")
	}
	if !strings.Contains(err.Error(), "--workspace") || !strings.Contains(err.Error(), "--machine") {
		t.Fatalf("the error does not say what to choose between: %v", err)
	}
}

func TestPackagesAddDefaultsToTheOnlyMachine(t *testing.T) {
	dir := configWithMachineAndWorkspace(t)

	if _, err := execute(t, "--config", dir, "packages", "add", "docker", "--yes"); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.Load(dir)
	if len(cfg.Machines[0].Packages) != 1 || cfg.Machines[0].Packages[0] != "docker" {
		t.Fatalf("got %#v", cfg.Machines[0].Packages)
	}
}

func TestPackagesAddSaysNothingChangedWhenItIsAlreadyThere(t *testing.T) {
	dir := configWithMachineAndWorkspace(t)

	if _, err := execute(t, "--config", dir, "packages", "add", "docker", "--machine", "main", "--yes"); err != nil {
		t.Fatal(err)
	}
	out, err := execute(t, "--config", dir, "packages", "add", "docker", "--machine", "main", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "already") {
		t.Fatalf("got %q", out)
	}

	cfg, _ := config.Load(dir)
	if len(cfg.Machines[0].Packages) != 1 {
		t.Fatalf("it was added twice: %#v", cfg.Machines[0].Packages)
	}
}

func TestPackagesAddWithCheckChangesNothing(t *testing.T) {
	dir := configWithMachineAndWorkspace(t)

	if _, err := execute(t, "--config", dir, "packages", "add", "docker", "--machine", "main", "--check"); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.Load(dir)
	if len(cfg.Machines[0].Packages) != 0 {
		t.Fatalf("a dry run wrote to the configuration: %#v", cfg.Machines[0].Packages)
	}
}

func TestPackagesAddKeepsTheCommentsAroundIt(t *testing.T) {
	dir := configWith(t, "# The main server.\nmachines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	if _, err := execute(t, "--config", dir, "packages", "add", "docker", "--yes"); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(dir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "# The main server.") {
		t.Fatalf("the comment was lost:\n%s", body)
	}
}

func TestPackagesRmTakesItOffTheTarget(t *testing.T) {
	dir := configWithPackagesInstalled(t)

	if _, err := execute(t, "--config", dir, "packages", "rm", "claude-code", "--workspace", "alice", "--yes"); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.Load(dir)
	if len(cfg.Workspaces[0].Packages) != 0 {
		t.Fatalf("got %#v", cfg.Workspaces[0].Packages)
	}
}

func TestPackagesRmSaysWhenItWasNotThere(t *testing.T) {
	dir := configWithMachineAndWorkspace(t)

	out, err := execute(t, "--config", dir, "packages", "rm", "docker", "--machine", "main", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not") {
		t.Fatalf("got %q", out)
	}
}

func TestPackagesAddNamesTheWorkspacesThatExist(t *testing.T) {
	dir := configWithMachineAndWorkspace(t)

	_, err := execute(t, "--config", dir, "packages", "add", "docker", "--workspace", "carol", "--yes")
	if err == nil {
		t.Fatal("an unknown workspace was accepted")
	}
	if !strings.Contains(err.Error(), "alice") {
		t.Fatalf("the error does not list the ones that exist: %v", err)
	}
}

func TestPackagesListShowsWhereEachOneIsInstalled(t *testing.T) {
	dir := configWithPackagesInstalled(t)

	out, err := execute(t, "--config", dir, "--format", "json", "packages", "list")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Packages []struct {
			Name      string   `json:"name"`
			Scope     string   `json:"scope"`
			Source    string   `json:"source"`
			Installed []string `json:"installed_on"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, out)
	}
	if len(got.Packages) != 3 {
		t.Fatalf("got %#v", got.Packages)
	}

	where := map[string][]string{}
	for _, p := range got.Packages {
		where[p.Name] = p.Installed
	}
	if len(where["docker"]) != 1 || where["docker"][0] != "machine main" {
		t.Fatalf("docker is installed on %#v", where["docker"])
	}
	if len(where["claude-code"]) != 1 || where["claude-code"][0] != "workspace alice" {
		t.Fatalf("claude-code is installed on %#v", where["claude-code"])
	}
	if len(where["caddy"]) != 0 {
		t.Fatalf("caddy is on nothing, and the list says %#v", where["caddy"])
	}
}

func TestPackagesListAsATableNamesEachPackageOnce(t *testing.T) {
	dir := configWithPackagesInstalled(t)

	out, err := execute(t, "--config", dir, "packages", "list")
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "claude-code") {
			rows++
		}
	}
	if rows != 1 {
		t.Fatalf("claude-code is on %d rows:\n%s", rows, out)
	}
	for _, want := range []string{"docker", "machine main", "workspace alice", "local"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the table leaves out %q:\n%s", want, out)
		}
	}
}

func TestPackagesListMarksAConfiguredPackageThatDoesNotExist(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [ghost]\n")

	out, err := execute(t, "--config", dir, "packages", "list")
	if err != nil {
		t.Fatal(err)
	}
	// Dropping it silently would leave `sync` to be the thing that finds out.
	if !strings.Contains(out, "ghost") || !strings.Contains(out, "missing") {
		t.Fatalf("a configured package nothing provides is not reported:\n%s", out)
	}
}
