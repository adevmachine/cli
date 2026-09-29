package secrets

import "testing"

func TestSetTargetThenFindTargetReturnsIt(t *testing.T) {
	dir := t.TempDir()

	if err := SetTarget(dir, Target{Workspace: "alice", Name: "API_KEY", EnvFile: "app/.env"}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := FindTarget(dir, "alice", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("the target was not found")
	}
	if got.EnvFile != "app/.env" {
		t.Fatalf("got %#v", got)
	}
}

func TestFindTargetReportsAbsence(t *testing.T) {
	_, ok, err := FindTarget(t.TempDir(), "alice", "MISSING")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("found a target that was never set")
	}
}

func TestSetTargetOverwritesAnExistingOne(t *testing.T) {
	dir := t.TempDir()

	if err := SetTarget(dir, Target{Workspace: "alice", Name: "API_KEY", EnvFile: "old/.env"}); err != nil {
		t.Fatal(err)
	}
	if err := SetTarget(dir, Target{Workspace: "alice", Name: "API_KEY", EnvFile: "new/.env"}); err != nil {
		t.Fatal(err)
	}
	got, _, err := FindTarget(dir, "alice", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if got.EnvFile != "new/.env" {
		t.Fatalf("got %#v", got)
	}
}

func TestTargetsIsSortedByWorkspaceThenName(t *testing.T) {
	dir := t.TempDir()

	for _, t2 := range []Target{
		{Workspace: "bob", Name: "B"},
		{Workspace: "alice", Name: "Z"},
		{Workspace: "alice", Name: "A"},
	} {
		if err := SetTarget(dir, t2); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Targets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %#v", got)
	}
	want := []string{"alice/A", "alice/Z", "bob/B"}
	for i, k := range want {
		if got[i].Key() != k {
			t.Fatalf("got %#v, want %v at position %d", got, want, i)
		}
	}
}

func TestRemoveTargetForgetsIt(t *testing.T) {
	dir := t.TempDir()
	if err := SetTarget(dir, Target{Workspace: "alice", Name: "API_KEY"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTarget(dir, "alice", "API_KEY"); err != nil {
		t.Fatal(err)
	}
	_, ok, err := FindTarget(dir, "alice", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("the target survived removal")
	}
}

func TestRemoveTargetOnAnEmptyStoreIsNotAnError(t *testing.T) {
	if err := RemoveTarget(t.TempDir(), "alice", "MISSING"); err != nil {
		t.Fatal(err)
	}
}

func TestTargetsOnAFreshDirectoryIsEmpty(t *testing.T) {
	got, err := Targets(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}
