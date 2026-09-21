package credentials

import (
	"slices"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
)

func TestResolveTakesTheOperatorsAnswerOverThePackagesRecommendation(t *testing.T) {
	gh := ghLogin().Credentials[0]

	scope, err := Resolve("dev", gh, map[string]string{"gh": config.CredentialOwn})
	if err != nil {
		t.Fatal(err)
	}
	if scope != packages.ScopeWorkspace {
		t.Fatalf("the operator asked for their own login, got %q", scope)
	}
}

func TestResolveFallsBackToThePackagesRecommendation(t *testing.T) {
	gh := ghLogin().Credentials[0]

	scope, err := Resolve("dev", gh, nil)
	if err != nil {
		t.Fatal(err)
	}
	if scope != packages.ScopeMachine {
		t.Fatalf("nobody said anything, so the package decides: %q", scope)
	}
}

func TestResolveRefusesToShareWhatTheToolSaysCannotTravel(t *testing.T) {
	bound := packages.Credential{
		Name: "vendor", Kind: packages.KindLogin, Scope: packages.ScopeWorkspace,
		Command: "vendor login", StoredAt: "~/.vendor/session",
	}

	scope, err := Resolve("vendor-cli", bound, map[string]string{"vendor": config.CredentialMachine})
	if err == nil {
		t.Fatal("asking to share a login that does not travel has to be refused")
	}
	if !strings.Contains(err.Error(), "vendor-cli") || !strings.Contains(err.Error(), "shareable") {
		t.Fatalf("the message has to name the package and say why: %v", err)
	}
	// Refusing has to leave the safe answer behind it, never a shared login
	// the tool cannot use.
	if scope != packages.ScopeWorkspace {
		t.Fatalf("got %q", scope)
	}
}

func TestResolveNeverSharesWhatCannotTravelEvenWhenThePackageSaysSo(t *testing.T) {
	contradictory := packages.Credential{
		Name: "vendor", Kind: packages.KindLogin, Scope: packages.ScopeMachine,
		Command: "vendor login", StoredAt: "~/.vendor/session",
	}

	scope, err := Resolve("vendor-cli", contradictory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if scope != packages.ScopeWorkspace {
		t.Fatalf("a recommendation cannot beat the fact, got %q", scope)
	}
}

func TestWantedGivesAWorkspaceThatOptedOutALoginOfItsOwn(t *testing.T) {
	plan := planWith(t, nil, map[string][]packages.Manifest{
		"alice": {ghLogin()},
		"bob":   {ghLogin()},
	})
	prefer(plan, "bob", map[string]string{"gh": config.CredentialOwn})

	var keys []string
	for _, d := range Wanted(plan) {
		keys = append(keys, Key(d))
	}
	// One shared login for alice, and one bob obtains for himself.
	if !slices.Equal(keys, []string{"gh", "bob/gh"}) {
		t.Fatalf("got %#v", keys)
	}
}

func TestSharingSkipsAWorkspaceThatOptedOut(t *testing.T) {
	plan := planWith(t, nil, map[string][]packages.Manifest{
		"alice": {ghLogin()},
		"bob":   {ghLogin()},
	})
	prefer(plan, "bob", map[string]string{"gh": config.CredentialOwn})

	shared, err := Sharing(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(shared) != 1 {
		t.Fatalf("got %#v", shared)
	}
	var into []string
	for _, account := range shared[0].Into {
		into = append(into, account.Workspace)
	}
	// The copy overwrites whatever is at stored_at. Copying it into bob would
	// destroy the account he logged in with, on the next sync, with nothing
	// saying why.
	if !slices.Equal(into, []string{"alice"}) {
		t.Fatalf("the shared login reached %#v", into)
	}
}

func TestSharingSaysWhereTheCopyComesFromAndWhereItLands(t *testing.T) {
	plan := planWith(t, nil, map[string][]packages.Manifest{"alice": {ghLogin()}})

	shared, err := Sharing(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(shared) != 1 {
		t.Fatalf("got %#v", shared)
	}
	if shared[0].From != "/etc/devmachine/gh/hosts.yml" {
		t.Fatalf("the master copy is at %q", shared[0].From)
	}
	if shared[0].StoredAt != "~/.config/gh/hosts.yml" {
		t.Fatalf("it lands at %q", shared[0].StoredAt)
	}
	if shared[0].Into[0].LinuxUser != "alice" {
		t.Fatalf("got %#v", shared[0].Into)
	}
}

func TestSharingLeavesAWorkspaceScopedLoginAlone(t *testing.T) {
	plan := planWith(t, nil, map[string][]packages.Manifest{"alice": {claudeLogin()}})

	shared, err := Sharing(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(shared) != 0 {
		t.Fatalf("a login nobody shares was copied anyway: %#v", shared)
	}
}

func TestSharingSharesAWorkspaceScopedLoginWhenTheOperatorAsks(t *testing.T) {
	plan := planWith(t, nil, map[string][]packages.Manifest{
		"alice": {claudeLogin()},
		"bob":   {claudeLogin()},
	})
	prefer(plan, "alice", map[string]string{"claude": config.CredentialMachine})
	prefer(plan, "bob", map[string]string{"claude": config.CredentialMachine})

	shared, err := Sharing(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(shared) != 1 || len(shared[0].Into) != 2 {
		t.Fatalf("somebody who works under one account shares one login: %#v", shared)
	}
}

func TestSharingRefusesToShareWhatCannotTravel(t *testing.T) {
	bound := packages.Manifest{
		Name: "vendor-cli", Scope: packages.ScopeWorkspace,
		Credentials: []packages.Credential{{
			Name: "vendor", Kind: packages.KindLogin, Scope: packages.ScopeWorkspace,
			Command: "vendor login", StoredAt: "~/.vendor/session",
		}},
	}
	plan := planWith(t, nil, map[string][]packages.Manifest{"alice": {bound}})
	prefer(plan, "alice", map[string]string{"vendor": config.CredentialMachine})

	if _, err := Sharing(plan); err == nil {
		t.Fatal("the refusal has to reach whoever asked, not be quietly dropped")
	}
}

// prefer sets one workspace's credential answers on an already resolved plan.
func prefer(plan packages.MachinePlan, workspace string, answers map[string]string) {
	for i := range plan.Workspaces {
		if plan.Workspaces[i].Target.Name == workspace {
			plan.Workspaces[i].Target.Credentials = answers
		}
	}
}
