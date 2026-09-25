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
