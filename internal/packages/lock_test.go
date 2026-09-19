package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adevmachine/cli/internal/config"
)

// planWith builds a plan straight from names, so a lock test says what it is
// about without going through a store.
func planWith(machine string, machinePackages []string, workspaces map[string][]string) MachinePlan {
	plan := MachinePlan{Machine: config.Machine{Name: machine}}
	plan.OnMachine = Resolved{
		Target:  Target{Kind: ScopeMachine, Name: machine, Packages: machinePackages},
		Ordered: foundFrom(machinePackages, ScopeMachine, SourceRelease),
	}
	for _, name := range sortedNames(workspaces) {
		plan.Workspaces = append(plan.Workspaces, Resolved{
			Target:  Target{Kind: ScopeWorkspace, Name: name, LinuxUser: name, Packages: workspaces[name]},
			Ordered: foundFrom(workspaces[name], ScopeWorkspace, SourceRelease),
		})
	}
	return plan
}

func foundFrom(packageNames []string, scope, source string) []Found {
	out := make([]Found, 0, len(packageNames))
	for _, name := range packageNames {
		out = append(out, Found{Manifest: Manifest{Format: 1, Name: name, Scope: scope}, Source: source})
	}
	return out
}

func sortedNames(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func storeStub(version, checksum string) *Store {
	return &Store{version: version, checksum: checksum}
}

func TestLockRecordsWhatWasAppliedWhere(t *testing.T) {
	configDir := t.TempDir()
	plan := planWith("main", []string{"base", "docker"}, map[string][]string{"alice": {"claude-code"}})

	lock := Lock{}.WithPlan(plan, storeStub("v1", "abc123"), time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC))
	if err := SaveLock(configDir, lock); err != nil {
		t.Fatal(err)
	}

	got, err := LoadLock(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Release != "v1" {
		t.Fatalf("release %q", got.Release)
	}
	if got.AppliedAt != "2026-09-18T12:00:00Z" {
		t.Fatalf("applied_at %q", got.AppliedAt)
	}
	if len(got.Machines["main"]) != 2 {
		t.Fatalf("machine entries %#v", got.Machines["main"])
	}
	if got.Machines["main"][0].Checksum != "abc123" {
		t.Fatalf("checksum %q", got.Machines["main"][0].Checksum)
	}
	if got.Machines["main"][0].Version != "v1" {
		t.Fatalf("version %q", got.Machines["main"][0].Version)
	}
	if len(got.Workspaces["alice"]) != 1 {
		t.Fatalf("workspace entries %#v", got.Workspaces)
	}
}

func TestLockKeepsOtherMachinesAlone(t *testing.T) {
	existing := Lock{Machines: map[string][]LockEntry{"sandbox": {{Name: "git", Source: SourceRelease}}}}
	plan := planWith("main", []string{"base"}, nil)

	got := existing.WithPlan(plan, storeStub("v1", "abc123"), time.Now())
	if len(got.Machines["sandbox"]) != 1 {
		t.Fatal("syncing one machine rewrote another's entry")
	}
	if len(existing.Machines) != 1 {
		t.Fatal("WithPlan wrote into the lock it was given")
	}
}

// A local package has nothing to pin, and saying otherwise would be a lie in
// a file whose whole job is being true.
func TestLockRecordsALocalPackageWithNothingToPin(t *testing.T) {
	plan := MachinePlan{
		Machine: config.Machine{Name: "main"},
		OnMachine: Resolved{
			Target:  Target{Kind: ScopeMachine, Name: "main", Packages: []string{"caddy"}},
			Ordered: foundFrom([]string{"caddy"}, ScopeMachine, SourceLocal),
		},
	}

	got := Lock{}.WithPlan(plan, storeStub("v1", "abc123"), time.Now())
	entry := got.Machines["main"][0]
	if entry.Source != SourceLocal {
		t.Fatalf("source %q", entry.Source)
	}
	if entry.Version != "" || entry.Checksum != "" {
		t.Fatalf("a local package was pinned: %#v", entry)
	}
}

// A workspace that had packages and now has none is a real change, and the
// lock has to show it rather than keep yesterday's list.
func TestLockClearsAWorkspaceThatNowDeclaresNothing(t *testing.T) {
	existing := Lock{Workspaces: map[string][]LockEntry{"alice": {{Name: "claude-code", Source: SourceRelease}}}}
	plan := planWith("main", nil, map[string][]string{"alice": {}})

	got := existing.WithPlan(plan, storeStub("v1", "abc123"), time.Now())
	if len(got.Workspaces["alice"]) != 0 {
		t.Fatalf("got %#v", got.Workspaces["alice"])
	}
}

func TestLoadLockOnAFreshConfigIsEmptyNotAnError(t *testing.T) {
	got, err := LoadLock(t.TempDir())
	if err != nil {
		t.Fatalf("a missing lock is the ordinary first state: %v", err)
	}
	if len(got.Machines) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestLoadLockSaysWhichFileItCouldNotRead(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, LockFile), "machines: [unclosed\n")

	if _, err := LoadLock(dir); err == nil {
		t.Fatal("a malformed lock was accepted")
	} else if !strings.Contains(err.Error(), LockFile) {
		t.Fatalf("the error does not name the file: %v", err)
	}
}

func TestSaveLockLeavesNoTemporaryFileBehind(t *testing.T) {
	dir := t.TempDir()
	if err := SaveLock(dir, Lock{}.WithPlan(planWith("main", []string{"base"}, nil), storeStub("v1", "abc"), time.Now())); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != LockFile {
		t.Fatalf("got %#v", entries)
	}
}
