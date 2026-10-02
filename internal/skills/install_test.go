package skills

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInstallWritesOneCanonicalCopyAndARelativeClaudeLink(t *testing.T) {
	home, source := fixture(t)
	result, err := (Installer{Home: home}).Install(Source{ID: "local:test", Root: source}, []Agent{AgentClaude, AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(home, ".agents", "skills", "example", "SKILL.md"))
	assertContents(t, filepath.Join(home, ".agents", "skills", "example", "references", "usage.md"), "usage")
	target, err := os.Readlink(filepath.Join(home, ".claude", "skills", "example"))
	if err != nil {
		t.Fatal(err)
	}
	if target != "../../.agents/skills/example" {
		t.Fatalf("target %q", target)
	}
	if result.Changed != 2 {
		t.Fatalf("got %#v", result)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	installer, source := fixtureInstaller(t)
	if _, err := installer.Install(source, []Agent{AgentClaude}); err != nil {
		t.Fatal(err)
	}
	second, err := installer.Install(source, []Agent{AgentClaude})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed != 0 {
		t.Fatalf("got %#v", second)
	}
}

func TestInstallKeepsOwnershipForASkillRemovedFromTheSource(t *testing.T) {
	installer, source := fixtureInstaller(t)
	writeSkill(t, source.Root, "second", "A second skill.")
	if _, err := installer.Install(source, []Agent{AgentClaude}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(source.Root, "example")); err != nil {
		t.Fatal(err)
	}

	if _, err := installer.Install(source, []Agent{AgentClaude}); err != nil {
		t.Fatal(err)
	}
	records, err := installer.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !slices.Equal(records[0].Skills, []string{"example", "second"}) {
		t.Fatalf("ownership lost after additive update: %#v", records)
	}
	if _, err := installer.Remove(source.ID, []string{"example"}); err != nil {
		t.Fatalf("the retained skill can no longer be removed explicitly: %v", err)
	}
}

func TestInstallDoesNotDeleteAnUnmanagedBackupNamedPath(t *testing.T) {
	installer, source := fixtureInstaller(t)
	if _, err := installer.Install(source, []Agent{AgentCodex}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(installer.Home, ".agents", "skills", "example.devmachine-backup")
	writeTestFile(t, filepath.Join(backup, "mine"), "keep")
	writeTestFile(t, filepath.Join(source.Root, "example", "SKILL.md"), `---
name: example
description: Updated example.
---
updated
`)

	if _, err := installer.Install(source, []Agent{AgentCodex}); err != nil {
		t.Fatal(err)
	}
	assertContents(t, filepath.Join(backup, "mine"), "keep")
}

func TestInstallRefusesAnUnmanagedCollision(t *testing.T) {
	installer, source := fixtureInstaller(t)
	writeTestFile(t, filepath.Join(installer.Home, ".agents", "skills", "example", "SKILL.md"), "mine")

	_, err := installer.Install(source, []Agent{AgentCodex})
	if err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("got %v", err)
	}
	assertContents(t, filepath.Join(installer.Home, ".agents", "skills", "example", "SKILL.md"), "mine")
}

func TestInstallRefusesANameOwnedByAnotherSource(t *testing.T) {
	installer, source := fixtureInstaller(t)
	if _, err := installer.Install(source, []Agent{AgentCodex}); err != nil {
		t.Fatal(err)
	}
	otherRoot := t.TempDir()
	writeSkill(t, otherRoot, "example", "A different example.")

	_, err := installer.Install(Source{ID: "local:other", Root: otherRoot}, []Agent{AgentCodex})
	if err == nil || !strings.Contains(err.Error(), source.ID) || !strings.Contains(err.Error(), "local:other") {
		t.Fatalf("got %v", err)
	}
}

func TestRemoveRefusesARecordOwnedByAnotherSource(t *testing.T) {
	installer, source := fixtureInstaller(t)
	if _, err := installer.Install(source, []Agent{AgentClaude}); err != nil {
		t.Fatal(err)
	}

	_, err := installer.Remove("local:other", []string{"example"})
	if err == nil || !strings.Contains(err.Error(), source.ID) {
		t.Fatalf("got %v", err)
	}
	assertFile(t, filepath.Join(installer.Home, ".agents", "skills", "example", "SKILL.md"))
	if _, err := os.Lstat(filepath.Join(installer.Home, ".claude", "skills", "example")); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveDeletesOnlyTheRequestedOwnedSkill(t *testing.T) {
	installer, source := fixtureInstaller(t)
	writeSkill(t, source.Root, "second", "A second skill.")
	if _, err := installer.Install(source, []Agent{AgentClaude}); err != nil {
		t.Fatal(err)
	}

	result, err := installer.Remove(source.ID, []string{"example"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 2 || !slices.Equal(result.Skills, []string{"example"}) {
		t.Fatalf("got %#v", result)
	}
	if _, err := os.Lstat(filepath.Join(installer.Home, ".agents", "skills", "example")); !os.IsNotExist(err) {
		t.Fatalf("example remains: %v", err)
	}
	assertFile(t, filepath.Join(installer.Home, ".agents", "skills", "second", "SKILL.md"))
}

func TestInstallLinksAntigravityAndClineButNotKimi(t *testing.T) {
	installer, source := fixtureInstaller(t)
	result, err := installer.Install(source, []Agent{AgentCline, AgentKimi, AgentAntigravity})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Agents, []Agent{AgentAntigravity, AgentKimi, AgentCline}) {
		t.Fatalf("agents %v", result.Agents)
	}
	for link, want := range map[string]string{
		filepath.Join(installer.Home, ".gemini", "antigravity-cli", "skills", "example"): "../../../.agents/skills/example",
		filepath.Join(installer.Home, ".cline", "skills", "example"):                     "../../.agents/skills/example",
	} {
		target, err := os.Readlink(link)
		if err != nil {
			t.Fatal(err)
		}
		if target != want {
			t.Fatalf("%s -> %q, want %q", link, target, want)
		}
		assertFile(t, filepath.Join(link, "SKILL.md"))
	}
	if _, err := os.Lstat(filepath.Join(installer.Home, ".kimi-code")); !os.IsNotExist(err) {
		t.Fatalf("kimi reads the canonical copy and needs no adapter: %v", err)
	}
}

func TestInstallRefusesAnUnmanagedLinkedAdapterPath(t *testing.T) {
	for agent, path := range map[Agent][]string{
		AgentClaude:      {".claude", "skills", "example"},
		AgentAntigravity: {".gemini", "antigravity-cli", "skills", "example"},
		AgentCline:       {".cline", "skills", "example"},
	} {
		t.Run(string(agent), func(t *testing.T) {
			installer, source := fixtureInstaller(t)
			writeTestFile(t, filepath.Join(append([]string{installer.Home}, append(path, "SKILL.md")...)...), "mine")

			_, err := installer.Install(source, []Agent{agent})
			want := string(agent) + " adapter for skill \"example\" collides with unmanaged path"
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("got %v, want %q", err, want)
			}
			if _, err := os.Lstat(filepath.Join(installer.Home, ".agents", "skills", "example")); !os.IsNotExist(err) {
				t.Fatalf("canonical copy written despite the collision: %v", err)
			}
		})
	}
}

func TestInstallRemovesTheLinkOfADroppedAgent(t *testing.T) {
	installer, source := fixtureInstaller(t)
	if _, err := installer.Install(source, []Agent{AgentAntigravity, AgentCline}); err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(source, []Agent{AgentCline}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(installer.Home, ".gemini", "antigravity-cli", "skills", "example")); !os.IsNotExist(err) {
		t.Fatalf("antigravity link remains: %v", err)
	}
	if _, err := os.Readlink(filepath.Join(installer.Home, ".cline", "skills", "example")); err != nil {
		t.Fatalf("cline link is gone: %v", err)
	}
}

func TestRemoveDeletesEveryManagedLink(t *testing.T) {
	installer, source := fixtureInstaller(t)
	if _, err := installer.Install(source, []Agent{AgentClaude, AgentAntigravity, AgentCline}); err != nil {
		t.Fatal(err)
	}
	result, err := installer.Remove(source.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 4 {
		t.Fatalf("got %#v", result)
	}
	for _, link := range [][]string{
		{".claude", "skills", "example"},
		{".gemini", "antigravity-cli", "skills", "example"},
		{".cline", "skills", "example"},
	} {
		if _, err := os.Lstat(filepath.Join(append([]string{installer.Home}, link...)...)); !os.IsNotExist(err) {
			t.Fatalf("%v remains: %v", link, err)
		}
	}
}

func TestInstallRejectsAnUnknownAgentListingEverySupportedOne(t *testing.T) {
	installer, source := fixtureInstaller(t)
	_, err := installer.Install(source, []Agent{"emacs"})
	if err == nil {
		t.Fatal("unknown agent accepted")
	}
	for _, agent := range []string{"claude", "codex", "opencode", "pi", "antigravity", "kimi", "cline"} {
		if !strings.Contains(err.Error(), agent) {
			t.Fatalf("%q is missing from %v", agent, err)
		}
	}
}

func fixture(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	source := t.TempDir()
	writeSkill(t, source, "example", "An example skill.")
	writeTestFile(t, filepath.Join(source, "example", "references", "usage.md"), "usage")
	return home, source
}

func fixtureInstaller(t *testing.T) (Installer, Source) {
	t.Helper()
	home, root := fixture(t)
	return Installer{Home: home, StateDir: filepath.Join(home, "state")}, Source{ID: "local:test", Root: root}
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("%s is not a file: %v", path, err)
	}
}

func assertContents(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != want {
		t.Fatalf("%s = %q, want %q", path, body, want)
	}
}
