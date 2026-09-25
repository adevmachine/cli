package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverFindsCompleteSkillDirectories(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "two", "Use for the second task.")
	writeSkill(t, root, "one", "Use for one task.")

	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "one" || got[1].Name != "two" {
		t.Fatalf("got %#v", got)
	}
	for _, skill := range got {
		if skill.Path != filepath.Join(root, skill.Name) {
			t.Fatalf("%s has path %q", skill.Name, skill.Path)
		}
	}
}

func TestDiscoverRejectsAnEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}

	if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), "escape") {
		t.Fatalf("got %v", err)
	}
}

func TestDiscoverRejectsASymlinkInsideASkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "one", "Use for one task.")
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "one", "references")); err != nil {
		t.Fatal(err)
	}

	if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), "references") {
		t.Fatalf("got %v", err)
	}
}

func writeSkill(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
