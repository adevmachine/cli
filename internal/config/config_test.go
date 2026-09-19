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
		t.Fatalf("got (%q, %q)", dir, source)
	}
}

func TestDirFallsBackToTheEnvironment(t *testing.T) {
	t.Setenv("DEVMACHINE_CONFIG", "/from/env")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	dir, source, _ := Dir("")
	if dir != "/from/env" || source != SourceEnv {
		t.Fatalf("got (%q, %q)", dir, source)
	}
}

func TestDirFallsBackToXDG(t *testing.T) {
	t.Setenv("DEVMACHINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	dir, source, _ := Dir("")
	if dir != filepath.Join("/xdg", "devmachine") || source != SourceXDG {
		t.Fatalf("got (%q, %q)", dir, source)
	}
}

func TestDirFallsBackToTheHomeDirectory(t *testing.T) {
	t.Setenv("DEVMACHINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/alice")

	dir, source, _ := Dir("")
	if dir != filepath.Join("/home/alice", ".config", "devmachine") || source != SourceDefault {
		t.Fatalf("got (%q, %q)", dir, source)
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

const twoMachines = `
machines:
  - name: main
    hosts:
      - tailscale:vps
      - 203.0.113.10
  - name: sandbox
    hosts:
      - 127.0.0.1
    port: 52862
    key: /keys/sandbox
workspaces:
  - name: alice
    machine: main
  - name: bob
    machine: sandbox
    user: bob-dev
domain: example.com
`

func TestLoadReadsMachinesInOrder(t *testing.T) {
	c, err := Load(writeConfig(t, twoMachines))
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	if len(c.Machines) != 2 {
		t.Fatalf("got %d machines", len(c.Machines))
	}
	m := c.Machines[0]
	if m.Name != "main" || len(m.Hosts) != 2 || m.Hosts[0].Address != "tailscale:vps" {
		t.Fatalf("first machine = %#v", m)
	}
}

func TestLoadDefaultsTheAdminUserAndPortPerMachine(t *testing.T) {
	c, _ := Load(writeConfig(t, twoMachines))

	if c.Machines[0].User != "root" || c.Machines[0].Port != 22 {
		t.Fatalf("main did not get the defaults: %#v", c.Machines[0])
	}
	if c.Machines[1].Port != 52862 {
		t.Fatalf("sandbox lost its explicit port: %d", c.Machines[1].Port)
	}
}

func TestAWorkspaceUsesItsOwnNameAsTheLinuxUser(t *testing.T) {
	c, _ := Load(writeConfig(t, twoMachines))

	w, err := c.Workspace("alice")
	if err != nil {
		t.Fatalf("Workspace returned %v", err)
	}
	if w.LinuxUser() != "alice" {
		t.Fatalf("LinuxUser = %q, want the workspace name", w.LinuxUser())
	}
}

func TestAWorkspaceCanOverrideTheLinuxUser(t *testing.T) {
	c, _ := Load(writeConfig(t, twoMachines))

	w, _ := c.Workspace("bob")
	if w.LinuxUser() != "bob-dev" {
		t.Fatalf("LinuxUser = %q, want the override", w.LinuxUser())
	}
}

func TestMachineForAWorkspaceResolvesThroughTheMapping(t *testing.T) {
	c, _ := Load(writeConfig(t, twoMachines))

	m, w, err := c.MachineFor("bob")
	if err != nil {
		t.Fatalf("MachineFor returned %v", err)
	}
	if m.Name != "sandbox" {
		t.Fatalf("bob resolved to machine %q, want sandbox", m.Name)
	}
	if w.LinuxUser() != "bob-dev" {
		t.Fatalf("workspace = %#v", w)
	}
}

func TestAnUnknownWorkspaceListsTheOnesThatExist(t *testing.T) {
	c, _ := Load(writeConfig(t, twoMachines))

	_, err := c.Workspace("nope")
	if err == nil {
		t.Fatal("expected an error for an unknown workspace")
	}
	for _, want := range []string{"alice", "bob"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not list %q: %v", want, err)
		}
	}
}

func TestMachineByNameFindsIt(t *testing.T) {
	c, _ := Load(writeConfig(t, twoMachines))

	m, err := c.Machine("sandbox")
	if err != nil {
		t.Fatalf("Machine returned %v", err)
	}
	if m.Port != 52862 {
		t.Fatalf("got %#v", m)
	}
}

func TestMachineWithNoNameRefusesToGuessBetweenSeveral(t *testing.T) {
	c, _ := Load(writeConfig(t, twoMachines))

	_, err := c.Machine("")
	if err == nil {
		t.Fatal("expected an error rather than a guess when there are several machines")
	}
	for _, want := range []string{"main", "sandbox", "--machine"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not say how to choose (%q): %v", want, err)
		}
	}
}

func TestMachineWithNoNameTakesTheOnlyOne(t *testing.T) {
	c, _ := Load(writeConfig(t, "machines:\n  - name: only\n    hosts: [203.0.113.10]\n"))

	m, err := c.Machine("")
	if err != nil {
		t.Fatalf("Machine returned %v", err)
	}
	if m.Name != "only" {
		t.Fatalf("got %q", m.Name)
	}
}

func TestValidateRejectsAConfigWithNoMachine(t *testing.T) {
	err := Config{Domain: "example.com"}.Validate()
	if err == nil || !strings.Contains(err.Error(), "machines") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRejectsAMachineWithNoName(t *testing.T) {
	c := Config{Machines: []Machine{{Hosts: []Host{{Address: "203.0.113.10"}}, User: "root", Port: 22}}}
	if err := c.Validate(); err == nil {
		t.Fatal("expected an error for a machine with no name")
	}
}

func TestValidateRejectsTwoMachinesWithTheSameName(t *testing.T) {
	c := Config{Machines: []Machine{
		{Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, User: "root", Port: 22},
		{Name: "main", Hosts: []Host{{Address: "203.0.113.11"}}, User: "root", Port: 22},
	}}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "main") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRejectsAMachineWithNoHost(t *testing.T) {
	c := Config{Machines: []Machine{{Name: "main", User: "root", Port: 22}}}
	if err := c.Validate(); err == nil {
		t.Fatal("expected an error for a machine with no host")
	}
}

func TestValidateRejectsAPortOutOfRange(t *testing.T) {
	for _, port := range []int{0, -1, 70000} {
		c := Config{Machines: []Machine{{
			Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, User: "root", Port: port,
		}}}
		if err := c.Validate(); err == nil {
			t.Fatalf("expected an error for port %d", port)
		}
	}
}

func TestValidateRejectsAWorkspaceOnAMachineThatDoesNotExist(t *testing.T) {
	c := Config{
		Machines:   []Machine{{Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, User: "root", Port: 22}},
		Workspaces: []Workspace{{Name: "alice", Machine: "ghost"}},
	}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("the error does not name the missing machine: %v", err)
	}
}

func TestAWorkspaceMayOmitItsMachineWhenThereIsOnlyOne(t *testing.T) {
	dir := writeConfig(t, "machines:\n  - name: only\n    hosts: [203.0.113.10]\nworkspaces:\n  - name: alice\n")

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate returned %v", err)
	}
	m, _, err := c.MachineFor("alice")
	if err != nil {
		t.Fatalf("MachineFor returned %v", err)
	}
	if m.Name != "only" {
		t.Fatalf("got %q", m.Name)
	}
}

func TestAWorkspaceMustNameItsMachineWhenThereAreSeveral(t *testing.T) {
	c := Config{
		Machines: []Machine{
			{Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, User: "root", Port: 22},
			{Name: "sandbox", Hosts: []Host{{Address: "127.0.0.1"}}, User: "root", Port: 22},
		},
		Workspaces: []Workspace{{Name: "alice"}},
	}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "alice") {
		t.Fatalf("the error does not name the workspace: %v", err)
	}
}

func TestValidateRejectsTwoWorkspacesWithTheSameName(t *testing.T) {
	c := Config{
		Machines: []Machine{{Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, User: "root", Port: 22}},
		Workspaces: []Workspace{
			{Name: "alice", Machine: "main"},
			{Name: "alice", Machine: "main"},
		},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected an error for a duplicated workspace name")
	}
}

func TestWorkspacesOnReturnsOnlyThatMachines(t *testing.T) {
	c, _ := Load(writeConfig(t, twoMachines))

	got := c.WorkspacesOn("sandbox")
	if len(got) != 1 || got[0].Name != "bob" {
		t.Fatalf("got %#v", got)
	}
}

func TestLoadSaysWhichFileIsMissing(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), FileName) {
		t.Fatalf("got %v", err)
	}
}

func TestLoadRejectsBrokenYAML(t *testing.T) {
	if _, err := Load(writeConfig(t, "machines: [unclosed\n")); err == nil {
		t.Fatal("expected an error for broken YAML")
	}
}

func TestLoadReadsThePackagePinAndTheLists(t *testing.T) {
	dir := writeConfig(t, `
machines:
  - name: main
    hosts: [203.0.113.10]
    packages: [base, docker]
workspaces:
  - name: alice
    packages: [claude-code]
packages: v1
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Packages != "v1" {
		t.Fatalf("pin is %q", cfg.Packages)
	}
	if len(cfg.Machines[0].Packages) != 2 || cfg.Machines[0].Packages[1] != "docker" {
		t.Fatalf("machine packages %#v", cfg.Machines[0].Packages)
	}
	if len(cfg.Workspaces[0].Packages) != 1 {
		t.Fatalf("workspace packages %#v", cfg.Workspaces[0].Packages)
	}
}

func TestValidateRefusesTheSamePackageTwiceOnOneTarget(t *testing.T) {
	c := Config{
		Machines: []Machine{{Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, Port: 22,
			Packages: []string{"docker", "docker"}}},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("a duplicate was accepted")
	}
	if !strings.Contains(err.Error(), "docker") || !strings.Contains(err.Error(), "main") {
		t.Fatalf("the error does not say what and where: %v", err)
	}
}

func TestValidateRefusesTheSamePackageTwiceOnAWorkspace(t *testing.T) {
	c := Config{
		Machines:   []Machine{{Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, Port: 22}},
		Workspaces: []Workspace{{Name: "alice", Packages: []string{"claude-code", "claude-code"}}},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("a duplicate was accepted")
	}
	if !strings.Contains(err.Error(), "claude-code") || !strings.Contains(err.Error(), "alice") {
		t.Fatalf("the error does not say what and where: %v", err)
	}
}

func TestValidateRefusesAPinThatIsABranch(t *testing.T) {
	c := Config{
		Machines: []Machine{{Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, Port: 22}},
		Packages: "main",
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("a branch name was accepted as a pin")
	}
	if !strings.Contains(err.Error(), "tag") {
		t.Fatalf("the error does not say what a pin is: %v", err)
	}
}

func TestConfigSaveKeepsComments(t *testing.T) {
	dir := writeConfig(t, `
# The main server.
machines:
  - name: main
    hosts: [203.0.113.10]
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Machines[0].Packages = []string{"base"}
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "# The main server.") {
		t.Fatalf("the comment was lost:\n%s", body)
	}
	if !strings.Contains(string(body), "base") {
		t.Fatalf("the package was not written:\n%s", body)
	}
}

func TestConfigSaveLeavesTheRestOfTheFileAlone(t *testing.T) {
	dir := writeConfig(t, `
machines:
  - name: main
    hosts: [203.0.113.10, 100.64.0.7]
workspaces:
  - name: alice
domain: example.com
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Workspaces[0].Packages = []string{"claude-code"}
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	saved, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Domain != "example.com" {
		t.Fatalf("domain is %q", saved.Domain)
	}
	if len(saved.Machines[0].Hosts) != 2 || saved.Machines[0].Hosts[1].Address != "100.64.0.7" {
		t.Fatalf("hosts %#v", saved.Machines[0].Hosts)
	}
	// Load fills in the admin user and the port, so writing the struct back
	// would put values in the file that nobody wrote.
	body, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "port") || strings.Contains(string(body), "user") {
		t.Fatalf("a default leaked into the file:\n%s", body)
	}
}

func TestConfigSaveRoundTripsTheListsAndThePin(t *testing.T) {
	dir := writeConfig(t, `
machines:
  - name: main
    hosts: [203.0.113.10]
    packages: [base, docker]
workspaces:
  - name: alice
    packages: [claude-code]
packages: v1
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Machines[0].Packages = []string{"base"}
	cfg.Workspaces[0].Packages = nil
	cfg.Packages = "v2"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	saved, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Machines[0].Packages) != 1 || saved.Machines[0].Packages[0] != "base" {
		t.Fatalf("machine packages %#v", saved.Machines[0].Packages)
	}
	if len(saved.Workspaces[0].Packages) != 0 {
		t.Fatalf("workspace packages %#v", saved.Workspaces[0].Packages)
	}
	if saved.Packages != "v2" {
		t.Fatalf("pin is %q", saved.Packages)
	}
}

func TestConfigSaveWritesAFileThatIsNotThereYet(t *testing.T) {
	dir := t.TempDir()
	c := Config{
		Machines: []Machine{{Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, Packages: []string{"base"}}},
		Packages: "v1",
	}
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}

	saved, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Machines) != 1 || saved.Machines[0].Packages[0] != "base" {
		t.Fatalf("got %#v", saved.Machines)
	}
}

func TestConfigSaveRefusesATargetTheFileDoesNotHave(t *testing.T) {
	dir := writeConfig(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Machines = append(cfg.Machines, Machine{Name: "spare", Hosts: []Host{{Address: "203.0.113.11"}}})

	err = Save(dir, cfg)
	if err == nil {
		t.Fatal("a machine that is not in the file was silently dropped")
	}
	if !strings.Contains(err.Error(), "spare") {
		t.Fatalf("the error does not say which one: %v", err)
	}
}

func configDirWith(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAddMachineKeepsEverythingElseAsItWasWritten(t *testing.T) {
	dir := configDirWith(t, `# the machine I bought first
machines:
  - name: main
    hosts: [203.0.113.10]
domain: example.com
`)

	err := AddMachine(dir, Machine{
		Name:  "sandbox",
		Hosts: []Host{{Address: "198.51.100.7"}},
		User:  "root",
		Port:  2222,
		Key:   "/keys/sandbox",
	})
	if err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	// A file that loses its comments the first time the CLI touches it is a
	// file people stop letting the CLI touch.
	if !strings.Contains(string(body), "# the machine I bought first") {
		t.Fatalf("the comment is gone:\n%s", body)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Machines) != 2 {
		t.Fatalf("machines = %#v", cfg.Machines)
	}
	added := cfg.Machines[1]
	if added.Name != "sandbox" || added.Port != 2222 || added.Key != "/keys/sandbox" {
		t.Fatalf("got %#v", added)
	}
	if cfg.Domain != "example.com" {
		t.Fatalf("domain = %q", cfg.Domain)
	}
}

func TestAddMachineRefusesANameThatIsTaken(t *testing.T) {
	dir := configDirWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	err := AddMachine(dir, Machine{Name: "main", Hosts: []Host{{Address: "198.51.100.7"}}})
	if err == nil {
		t.Fatal("two machines were allowed the same name")
	}
	if !strings.Contains(err.Error(), "main") {
		t.Fatalf("the error does not name it: %v", err)
	}
}

func TestAddMachineNeedsAConfigurationToAddTo(t *testing.T) {
	err := AddMachine(t.TempDir(), Machine{Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}})
	if err == nil {
		t.Fatal("it wrote a configuration out of nothing")
	}
	if !strings.Contains(err.Error(), "setup") {
		t.Fatalf("the error does not say where to start: %v", err)
	}
}

func TestRemoveMachineTakesItOutAndLeavesTheRest(t *testing.T) {
	dir := configDirWith(t, `machines:
  - name: main
    hosts: [203.0.113.10]
  - name: sandbox
    hosts: [198.51.100.7]
domain: example.com
`)

	if err := RemoveMachine(dir, "sandbox"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Machines) != 1 || cfg.Machines[0].Name != "main" {
		t.Fatalf("machines = %#v", cfg.Machines)
	}
	if cfg.Domain != "example.com" {
		t.Fatalf("domain = %q", cfg.Domain)
	}
}

func TestRemoveMachineSaysWhichOnesThereAre(t *testing.T) {
	dir := configDirWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	err := RemoveMachine(dir, "absent")
	if err == nil {
		t.Fatal("it removed a machine that is not configured")
	}
	if !strings.Contains(err.Error(), "main") {
		t.Fatalf("the error does not say what is there: %v", err)
	}
}

func TestRemoveMachineRefusesToOrphanAWorkspace(t *testing.T) {
	// A workspace pointing at a machine that is gone is a configuration that
	// no longer loads, and the person finds out on the next command.
	dir := configDirWith(t, `machines:
  - name: main
    hosts: [203.0.113.10]
  - name: sandbox
    hosts: [198.51.100.7]
workspaces:
  - name: alice
    machine: sandbox
`)

	err := RemoveMachine(dir, "sandbox")
	if err == nil {
		t.Fatal("it left a workspace pointing at nothing")
	}
	if !strings.Contains(err.Error(), "alice") {
		t.Fatalf("the error does not name the workspace: %v", err)
	}

	cfg, _ := Load(dir)
	if len(cfg.Machines) != 2 {
		t.Fatal("it removed the machine anyway")
	}
}

func TestSettingsForStripsThePackagePrefix(t *testing.T) {
	got := SettingsFor(map[string]any{
		"caddy.email":   "someone@example.com",
		"base.timezone": "UTC",
	}, "caddy")

	if len(got) != 1 || got["email"] != "someone@example.com" {
		t.Fatalf("got %#v", got)
	}
}

func TestSettingsForKeepsADottedKeyInsideAPackage(t *testing.T) {
	// A package may want `claude-plugins.marketplace.url`. Only the first
	// segment is the package name.
	got := SettingsFor(map[string]any{"a.b.c": 1}, "a")
	if got["b.c"] != 1 {
		t.Fatalf("got %#v", got)
	}
}

func TestSettingsForIgnoresAnotherPackage(t *testing.T) {
	got := SettingsFor(map[string]any{"base.timezone": "UTC"}, "caddy")
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestLoadReadsTheSettingsOfBothKindsOfTarget(t *testing.T) {
	dir := writeConfig(t, `
machines:
  - name: main
    hosts: [203.0.113.10]
    packages: [base]
    settings:
      base.timezone: America/Sao_Paulo
workspaces:
  - name: alice
    packages: [claude-plugins]
    settings:
      claude-plugins.plugins: [their-plugin]
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Machines[0].Settings["base.timezone"] != "America/Sao_Paulo" {
		t.Fatalf("got %#v", c.Machines[0].Settings)
	}
	if got, ok := c.Workspaces[0].Settings["claude-plugins.plugins"].([]any); !ok || len(got) != 1 {
		t.Fatalf("got %#v", c.Workspaces[0].Settings)
	}
}

func TestValidateRefusesASettingWithNoPackagePrefix(t *testing.T) {
	c := Config{
		Machines: []Machine{{
			Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, Port: 22,
			Settings: map[string]any{"timezone": "UTC"},
		}},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("a setting belonging to nothing was accepted")
	}
	if !strings.Contains(err.Error(), "<package>.<name>") {
		t.Fatalf("the error does not give the shape: %v", err)
	}
}

func TestValidateRefusesASettingForAPackageTheTargetDoesNotHave(t *testing.T) {
	c := Config{
		Machines: []Machine{{
			Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, Port: 22,
			Packages: []string{"base"},
			Settings: map[string]any{"caddy.email": "x@example.com"},
		}},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("a setting for a package nobody installed was accepted")
	}
	// A typo in a package name would otherwise be silent: the value simply
	// never reaches anything, and the recipe keeps its default.
	if !strings.Contains(err.Error(), "caddy") {
		t.Fatalf("the error does not name it: %v", err)
	}
}

func TestValidateRefusesAWorkspaceSettingForAPackageItDoesNotHave(t *testing.T) {
	c := Config{
		Machines: []Machine{{Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, Port: 22}},
		Workspaces: []Workspace{{
			Name: "alice", Packages: []string{"zsh"},
			Settings: map[string]any{"claude-plugins.marketplace": "example.com/plugins"},
		}},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("a setting for a package the workspace does not have was accepted")
	}
	if !strings.Contains(err.Error(), "claude-plugins") || !strings.Contains(err.Error(), "alice") {
		t.Fatalf("the error does not say what and where: %v", err)
	}
}

func TestValidateAcceptsASettingForAPackageTheTargetHas(t *testing.T) {
	c := Config{
		Machines: []Machine{{
			Name: "main", Hosts: []Host{{Address: "203.0.113.10"}}, Port: 22,
			Packages: []string{"base", "caddy"},
			Settings: map[string]any{"caddy.email": "someone@example.com"},
		}},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
