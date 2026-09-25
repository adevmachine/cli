package skills

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestListReturnsPersistedSourcesInStableOrder(t *testing.T) {
	installer, source := fixtureInstaller(t)
	if _, err := installer.Install(Source{ID: "local:z", Root: source.Root}, []Agent{AgentCodex}); err != nil {
		t.Fatal(err)
	}
	otherRoot := t.TempDir()
	writeSkill(t, otherRoot, "another", "Another skill.")
	if _, err := installer.Install(Source{ID: "local:a", Root: otherRoot}, []Agent{AgentClaude}); err != nil {
		t.Fatal(err)
	}

	got, err := installer.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Source != "local:a" || got[1].Source != "local:z" {
		t.Fatalf("got %#v", got)
	}
	if !slices.Equal(got[0].Skills, []string{"another"}) || !slices.Equal(got[0].Agents, []Agent{AgentClaude}) {
		t.Fatalf("got %#v", got[0])
	}
	if matches, _ := filepath.Glob(filepath.Join(installer.StateDir, "*.json")); len(matches) != 2 {
		t.Fatalf("state files %#v", matches)
	}
}
