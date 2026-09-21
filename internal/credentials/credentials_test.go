package credentials

import (
	"slices"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
)

func TestEnvFileIsTheContractTheNextVersionReads(t *testing.T) {
	// A published DNS provider sources this exact path before it runs, so a
	// provider released today has to keep working against a CLI released next
	// year. Changing it breaks every provider that already exists.
	if got := EnvFile("hostinger"); got != "/etc/devmachine/hostinger/env" {
		t.Fatalf("got %q", got)
	}
	if got := MachineDir("hostinger"); got != "/etc/devmachine/hostinger" {
		t.Fatalf("got %q", got)
	}
}

func TestWantedListsAMachineCredentialOnce(t *testing.T) {
	// One `gh` session serves the whole machine. Two workspaces asking for it
	// is one thing to obtain, not two.
	plan := planWith(t,
		[]packages.Manifest{},
		map[string][]packages.Manifest{
			"alice": {ghLogin()},
			"bob":   {ghLogin()},
		})

	got := Wanted(plan)
	if len(got) != 1 {
		t.Fatalf("got %d credentials: %#v", len(got), got)
	}
	if got[0].Workspace != "" {
		t.Fatalf("a machine credential was tied to workspace %q", got[0].Workspace)
	}
	if Key(got[0]) != "gh" {
		t.Fatalf("got key %q", Key(got[0]))
	}
}

func TestWantedListsAWorkspaceCredentialOncePerWorkspace(t *testing.T) {
	// Claude accounts differ between workspaces, so there is no master to copy
	// and each one logs in for itself.
	plan := planWith(t,
		[]packages.Manifest{},
		map[string][]packages.Manifest{
			"alice": {claudeLogin()},
			"bob":   {claudeLogin()},
		})

	var keys []string
	for _, d := range Wanted(plan) {
		keys = append(keys, Key(d))
	}
	if !slices.Equal(keys, []string{"alice/claude", "bob/claude"}) {
		t.Fatalf("got %#v", keys)
	}
}

func TestWantedSaysWhichPackageAskedForIt(t *testing.T) {
	plan := planWith(t,
		[]packages.Manifest{},
		map[string][]packages.Manifest{"alice": {claudeLogin()}})

	got := Wanted(plan)
	if len(got) != 1 || got[0].Package != "claude-code" {
		t.Fatalf("got %#v", got)
	}
	if got[0].Command != "claude /login" {
		t.Fatalf("the declaration did not travel: %#v", got[0].Credential)
	}
}

func TestWantedListsAMachinePackagesCredential(t *testing.T) {
	plan := planWith(t, []packages.Manifest{hostingerToken()}, nil)

	got := Wanted(plan)
	if len(got) != 1 || Key(got[0]) != "hostinger" || got[0].Package != "hostinger" {
		t.Fatalf("got %#v", got)
	}
}

func TestWantedIsInAStableOrder(t *testing.T) {
	// `credentials list` and `doctor` print this. A map iteration would make
	// the same machine give a different report every run.
	plan := planWith(t,
		[]packages.Manifest{hostingerToken()},
		map[string][]packages.Manifest{
			"bob":   {claudeLogin(), ghLogin()},
			"alice": {claudeLogin()},
		})

	var keys []string
	for _, d := range Wanted(plan) {
		keys = append(keys, Key(d))
	}
	if !slices.Equal(keys, []string{"hostinger", "gh", "alice/claude", "bob/claude"}) {
		t.Fatalf("got %#v", keys)
	}
}

func TestWantedKeepsTheWorkspacesLinuxAccount(t *testing.T) {
	// The account is what a delivery and a lookup need: only the CLI knows
	// whose home `~` means.
	plan := planWith(t, nil, map[string][]packages.Manifest{"alice": {claudeLogin()}})
	plan.Workspaces[0].Target.LinuxUser = "alice-dev"

	got := Wanted(plan)
	if len(got) != 1 || got[0].LinuxUser != "alice-dev" {
		t.Fatalf("got %#v", got)
	}
}

// planWith builds a plan with the given manifests on the machine and on each
// workspace. It writes the plan directly rather than resolving a store: the
// question here is what is declared, not where it was fetched from.
func planWith(t *testing.T, onMachine []packages.Manifest,
	workspaces map[string][]packages.Manifest) packages.MachinePlan {
	t.Helper()

	plan := packages.MachinePlan{
		Machine: config.Machine{Name: "main"},
		OnMachine: packages.Resolved{
			Target: packages.Target{Kind: packages.ScopeMachine, Name: "main"},
		},
	}
	for _, m := range onMachine {
		plan.OnMachine.Ordered = append(plan.OnMachine.Ordered, packages.Found{Manifest: m})
	}

	names := make([]string, 0, len(workspaces))
	for name := range workspaces {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		resolved := packages.Resolved{
			Target: packages.Target{Kind: packages.ScopeWorkspace, Name: name, LinuxUser: name},
		}
		for _, m := range workspaces[name] {
			resolved.Ordered = append(resolved.Ordered, packages.Found{Manifest: m})
		}
		plan.Workspaces = append(plan.Workspaces, resolved)
	}
	return plan
}

func claudeLogin() packages.Manifest {
	return packages.Manifest{
		Name: "claude-code", Scope: packages.ScopeWorkspace,
		Credentials: []packages.Credential{{
			Name:  "claude",
			Kind:  packages.KindManual,
			Scope: packages.ScopeWorkspace,
			// The session file does copy; accounts usually differ between
			// workspaces, which is a different thing and the operator's call.
			Shareable: true,
			Command:   "claude /login",
			StoredAt:  "~/.claude/.credentials.json",
		}},
	}
}

func ghLogin() packages.Manifest {
	return packages.Manifest{
		Name: "gh-login", Scope: packages.ScopeWorkspace,
		Credentials: []packages.Credential{{
			Name:      "gh",
			Kind:      packages.KindManual,
			Scope:     packages.ScopeMachine,
			Shareable: true,
			Command:   "gh auth login",
			StoredAt:  "~/.config/gh/hosts.yml",
		}},
	}
}

func hostingerToken() packages.Manifest {
	return packages.Manifest{
		Name: "hostinger", Scope: packages.ScopeMachine,
		Credentials: []packages.Credential{{
			Name:  "hostinger",
			Kind:  packages.KindSecret,
			Scope: packages.ScopeMachine,
			Env:   "HOSTINGER_TOKEN",
		}},
	}
}
