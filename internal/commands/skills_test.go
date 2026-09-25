package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
)

func TestSkillsAddInstallsTheOfficialPackageWithoutDialing(t *testing.T) {
	dir, home := skillConfig(t, "v9")
	writeSkillPackage(t, filepath.Join(packages.CacheDir(dir, "v9"), "packages"), "devmachine-skills", "use-devmachine", "Use Devmachine.")
	forbidDial(t)

	out, err := executeWithHome(t, home, "--config", dir, "skills", "add", "--agent", "claude", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	assertCommandFile(t, filepath.Join(home, ".agents", "skills", "use-devmachine", "SKILL.md"))
	if !strings.Contains(out, "use-devmachine") {
		t.Fatalf("got %q", out)
	}
}

func TestSkillsAddWithoutAPinExplainsSetupAndNeverDials(t *testing.T) {
	dir := configWith(t, "machines: []\n")
	forbidDial(t)

	_, err := execute(t, "--config", dir, "skills", "add", "--agent", "codex", "--yes")
	if err == nil || !strings.Contains(err.Error(), "package release") {
		t.Fatalf("got %v", err)
	}
}

func TestSkillsAddOfficialBypassesALocalOverride(t *testing.T) {
	dir, home := skillConfig(t, "v9")
	writeSkillPackage(t, filepath.Join(packages.CacheDir(dir, "v9"), "packages"), "devmachine-skills", "use-devmachine", "Published copy.")
	writeSkillPackage(t, packages.LocalDir(dir), "devmachine-skills", "use-devmachine", "Local override.")

	if _, err := executeWithHome(t, home, "--config", dir, "skills", "add", "--agent", "codex", "--yes"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "use-devmachine", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Published copy.") || strings.Contains(string(body), "Local override.") {
		t.Fatalf("installed wrong source:\n%s", body)
	}
}

func TestSkillsAddPackageRequiresALocalPackage(t *testing.T) {
	dir, home := skillConfig(t, "v9")
	writeSkillPackage(t, filepath.Join(packages.CacheDir(dir, "v9"), "packages"), "shared-skills", "shared", "Shared.")

	_, err := executeWithHome(t, home, "--config", dir, "skills", "add", "--package", "shared-skills", "--agent", "codex", "--yes")
	if err == nil || !strings.Contains(err.Error(), "local package") {
		t.Fatalf("got %v", err)
	}
}

func TestSkillsListUpdateAndRemoveStayLocal(t *testing.T) {
	dir, home := skillConfig(t, "v9")
	writeSkillPackage(t, packages.LocalDir(dir), "global-skills", "workflow", "First version.")
	forbidDial(t)

	if _, err := executeWithHome(t, home, "--config", dir, "skills", "add", "--package", "global-skills", "--agent", "claude", "--yes"); err != nil {
		t.Fatal(err)
	}
	out, err := executeWithHome(t, home, "--config", dir, "--format", "json", "skills", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"source": "local:global-skills"`) || !strings.Contains(out, `"workflow"`) {
		t.Fatalf("got %s", out)
	}

	writeSkillPackage(t, packages.LocalDir(dir), "global-skills", "workflow", "Second version.")
	if _, err := executeWithHome(t, home, "--config", dir, "skills", "update", "--yes"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "workflow", "SKILL.md"))
	if err != nil || !strings.Contains(string(body), "Second version.") {
		t.Fatalf("update: %v\n%s", err, body)
	}

	if _, err := executeWithHome(t, home, "--config", dir, "skills", "remove", "workflow", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "workflow")); !os.IsNotExist(err) {
		t.Fatalf("skill remains: %v", err)
	}
}

func skillConfig(t *testing.T, version string) (string, string) {
	t.Helper()
	dir := configWith(t, "machines: []\npackages: "+version+"\n")
	writeCommandFile(t, filepath.Join(packages.CacheDir(dir, version), ".checksum"), "fixture\n")
	return dir, t.TempDir()
}

func writeSkillPackage(t *testing.T, parent, packageName, skillName, description string) {
	t.Helper()
	dir := filepath.Join(parent, packageName)
	manifest := "format: 1\nname: " + packageName + "\nscope: workspace\nsummary: Test skills.\nskills:\n  path: skills\n"
	writeCommandFile(t, filepath.Join(dir, "package.yml"), manifest)
	writeCommandFile(t, filepath.Join(dir, "tasks", "main.yml"), "---\n[]\n")
	body := "---\nname: " + skillName + "\ndescription: " + description + "\n---\n\n# " + skillName + "\n"
	writeCommandFile(t, filepath.Join(dir, "skills", skillName, "SKILL.md"), body)
}

func executeWithHome(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	return execute(t, args...)
}

func forbidDial(t *testing.T) {
	t.Helper()
	dial = func(context.Context, config.Machine, string) (remote.Client, string, error) {
		t.Fatal("a local skills command tried to dial a machine")
		return nil, "", nil
	}
	t.Cleanup(func() { dial = remote.Dial })
}

func writeCommandFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertCommandFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("%s is not a file: %v", path, err)
	}
}
