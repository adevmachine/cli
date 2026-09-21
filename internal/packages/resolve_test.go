package packages

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
)

// storeWith builds a store whose local directory holds these manifests.
func storeWith(t *testing.T, manifests map[string]string) *Store {
	t.Helper()
	configDir := t.TempDir()
	for name, body := range manifests {
		writePackageAt(t, filepath.Join(LocalDir(configDir), name), body)
	}
	store, err := Open(context.Background(), configDir, "")
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func names(found []Found) []string {
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, f.Manifest.Name)
	}
	return out
}

func assertBefore(t *testing.T, order []string, first, second string) {
	t.Helper()
	a, b := -1, -1
	for i, name := range order {
		switch name {
		case first:
			a = i
		case second:
			b = i
		}
	}
	if a < 0 || b < 0 {
		t.Fatalf("%q or %q is missing from %v", first, second, order)
	}
	if a > b {
		t.Fatalf("%q runs after %q in %v", first, second, order)
	}
}

func TestResolvePutsNeedsFirst(t *testing.T) {
	store := storeWith(t, map[string]string{
		"base":     "format: 1\nname: base\nscope: machine\nsummary: a\n",
		"docker":   "format: 1\nname: docker\nscope: machine\nsummary: b\nneeds: [base]\n",
		"caddy":    "format: 1\nname: caddy\nscope: machine\nsummary: c\nneeds: [firewall]\n",
		"firewall": "format: 1\nname: firewall\nscope: machine\nsummary: d\nneeds: [base]\n",
	})
	machine := config.Machine{Name: "main", Packages: []string{"caddy", "docker"}}

	plan, err := ResolveMachine(store, config.Config{Machines: []config.Machine{machine}}, machine, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	got := names(plan.OnMachine.Ordered)
	assertBefore(t, got, "base", "docker")
	assertBefore(t, got, "base", "firewall")
	assertBefore(t, got, "firewall", "caddy")
}

// Ordering comes from `needs` and from nothing else. Two configurations that
// list the same packages in a different order produce the same run.
func TestResolveDoesNotDependOnTheOrderOfTheList(t *testing.T) {
	manifests := map[string]string{
		"base":   "format: 1\nname: base\nscope: machine\nsummary: a\n",
		"docker": "format: 1\nname: docker\nscope: machine\nsummary: b\nneeds: [base]\n",
		"git":    "format: 1\nname: git\nscope: machine\nsummary: c\n",
	}
	one := config.Machine{Name: "main", Packages: []string{"docker", "git"}}
	other := config.Machine{Name: "main", Packages: []string{"git", "docker"}}

	first, err := ResolveMachine(storeWith(t, manifests), config.Config{Machines: []config.Machine{one}}, one, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveMachine(storeWith(t, manifests), config.Config{Machines: []config.Machine{other}}, other, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names(first.OnMachine.Ordered), ",") != strings.Join(names(second.OnMachine.Ordered), ",") {
		t.Fatalf("%v and %v", names(first.OnMachine.Ordered), names(second.OnMachine.Ordered))
	}
}

func TestResolveRefusesACycle(t *testing.T) {
	store := storeWith(t, map[string]string{
		"a": "format: 1\nname: a\nscope: machine\nsummary: x\nneeds: [b]\n",
		"b": "format: 1\nname: b\nscope: machine\nsummary: x\nneeds: [a]\n",
	})
	machine := config.Machine{Name: "main", Packages: []string{"a"}}

	_, err := ResolveMachine(store, config.Config{Machines: []config.Machine{machine}}, machine, "0.2.0")
	if err == nil {
		t.Fatal("a cycle was accepted")
	}
	if !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
		t.Fatalf("the error does not name the cycle: %v", err)
	}
}

func TestResolveRefusesAWorkspacePackageOnAMachine(t *testing.T) {
	store := storeWith(t, map[string]string{
		"claude-code": "format: 1\nname: claude-code\nscope: workspace\nsummary: x\n",
	})
	machine := config.Machine{Name: "main", Packages: []string{"claude-code"}}

	_, err := ResolveMachine(store, config.Config{Machines: []config.Machine{machine}}, machine, "0.2.0")
	if err == nil {
		t.Fatal("a workspace package was installed on a machine")
	}
	if !strings.Contains(err.Error(), "workspace") || !strings.Contains(err.Error(), "--workspace") {
		t.Fatalf("the error does not say how to fix it: %v", err)
	}
}

func TestResolveRefusesAMachinePackageOnAWorkspace(t *testing.T) {
	store := storeWith(t, map[string]string{
		"docker": "format: 1\nname: docker\nscope: machine\nsummary: x\n",
	})
	machine := config.Machine{Name: "main"}
	cfg := config.Config{
		Machines:   []config.Machine{machine},
		Workspaces: []config.Workspace{{Name: "alice", Packages: []string{"docker"}}},
	}

	_, err := ResolveMachine(store, cfg, machine, "0.2.0")
	if err == nil {
		t.Fatal("a machine package was installed on a workspace")
	}
	if !strings.Contains(err.Error(), "--machine") {
		t.Fatalf("the error does not say how to fix it: %v", err)
	}
}

func TestResolveRefusesARecipeThisCLICannotRead(t *testing.T) {
	store := storeWith(t, map[string]string{
		"future": "format: 1\nname: future\nscope: machine\nsummary: x\nrequires:\n  cli: \">= 9.0.0\"\n",
	})
	machine := config.Machine{Name: "main", Packages: []string{"future"}}

	_, err := ResolveMachine(store, config.Config{Machines: []config.Machine{machine}}, machine, "0.2.0")
	if err == nil {
		t.Fatal("a recipe from the future was accepted")
	}
	if !strings.Contains(err.Error(), "9.0.0") {
		t.Fatalf("the error does not say which version would read it: %v", err)
	}
}

func TestResolveRefusesAnExtensionPointNobodyProvides(t *testing.T) {
	store := storeWith(t, map[string]string{
		"sharing": "format: 1\nname: sharing\nscope: workspace\nsummary: x\nextends:\n  caddy.sites.d: files/sharing.caddy\n",
	})
	machine := config.Machine{Name: "main"}
	cfg := config.Config{
		Machines:   []config.Machine{machine},
		Workspaces: []config.Workspace{{Name: "alice", Packages: []string{"sharing"}}},
	}

	_, err := ResolveMachine(store, cfg, machine, "0.2.0")
	if err == nil {
		t.Fatal("an extension into a package that is not installed was accepted")
	}
	if !strings.Contains(err.Error(), "caddy is not installed") {
		t.Fatalf("the error does not say why: %v", err)
	}
}

func TestResolveWiresAnExtensionToTheProvidedPath(t *testing.T) {
	store := storeWith(t, map[string]string{
		"caddy":   "format: 1\nname: caddy\nscope: machine\nsummary: x\nprovides:\n  sites.d: /etc/caddy/sites.d\n",
		"sharing": "format: 1\nname: sharing\nscope: workspace\nsummary: x\nextends:\n  caddy.sites.d: files/sharing.caddy\n",
	})
	machine := config.Machine{Name: "main", Packages: []string{"caddy"}}
	cfg := config.Config{
		Machines:   []config.Machine{machine},
		Workspaces: []config.Workspace{{Name: "alice", Packages: []string{"sharing"}}},
	}

	plan, err := ResolveMachine(store, cfg, machine, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Extensions) != 1 {
		t.Fatalf("got %#v", plan.Extensions)
	}
	e := plan.Extensions[0]
	if e.Into != "/etc/caddy/sites.d" {
		t.Fatalf("wired to %q", e.Into)
	}
	if e.From != "sharing" || e.Source != "files/sharing.caddy" || e.Point != "caddy.sites.d" {
		t.Fatalf("got %#v", e)
	}
	if e.Target != "alice" {
		t.Fatalf("the extension does not say which target contributed it: %#v", e)
	}
}

func TestResolveCarriesEachWorkspaceAndItsLinuxAccount(t *testing.T) {
	store := storeWith(t, map[string]string{
		"claude-code": "format: 1\nname: claude-code\nscope: workspace\nsummary: x\n",
	})
	machine := config.Machine{Name: "main"}
	cfg := config.Config{
		Machines:   []config.Machine{machine},
		Workspaces: []config.Workspace{{Name: "alice", User: "alice-dev", Packages: []string{"claude-code"}}},
	}

	plan, err := ResolveMachine(store, cfg, machine, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Workspaces) != 1 {
		t.Fatalf("got %#v", plan.Workspaces)
	}
	target := plan.Workspaces[0].Target
	if target.Kind != ScopeWorkspace || target.Name != "alice" || target.LinuxUser != "alice-dev" {
		t.Fatalf("got %#v", target)
	}
}

func TestResolveNamesAPackageThatDoesNotExist(t *testing.T) {
	store := storeWith(t, map[string]string{
		"base": "format: 1\nname: base\nscope: machine\nsummary: x\n",
	})
	machine := config.Machine{Name: "main", Packages: []string{"absent"}}

	_, err := ResolveMachine(store, config.Config{Machines: []config.Machine{machine}}, machine, "0.2.0")
	if err == nil {
		t.Fatal("a package that does not exist was resolved")
	}
	if !strings.Contains(err.Error(), "absent") {
		t.Fatalf("the error does not name it: %v", err)
	}
}

func TestResolveCarriesTheCredentialAnswersTheWorkspaceEndsUpWith(t *testing.T) {
	store := storeWith(t, map[string]string{
		"dev": "format: 1\nname: dev\nscope: workspace\nsummary: x\n",
	})
	machine := config.Machine{Name: "main"}
	cfg := config.Config{
		Machines:    []config.Machine{machine},
		Credentials: map[string]string{"gh": config.CredentialMachine},
		Workspaces: []config.Workspace{
			{Name: "alice", Packages: []string{"dev"}},
			{Name: "bob", Packages: []string{"dev"}, Credentials: map[string]string{"gh": config.CredentialOwn}},
		},
	}

	plan, err := ResolveMachine(store, cfg, machine, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Workspaces[0].Target.Credentials["gh"]; got != config.CredentialMachine {
		t.Fatalf("alice said nothing, so the setup's default stands: %q", got)
	}
	if got := plan.Workspaces[1].Target.Credentials["gh"]; got != config.CredentialOwn {
		t.Fatalf("bob asked for his own: %q", got)
	}
}
