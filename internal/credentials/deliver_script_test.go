package credentials

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/packages"
)

func workspaceFile(user, path string) Declared {
	return Declared{
		Credential: packages.Credential{
			Name: "robot", Kind: packages.KindFile, Scope: packages.ScopeWorkspace, Path: path,
		},
		Package: "robot", Workspace: "alice", LinuxUser: user,
	}
}

func workspaceSecret(user string) Declared {
	d := secret("sentry", "SENTRY_TOKEN")
	d.Scope = packages.ScopeWorkspace
	d.Workspace, d.LinuxUser = "alice", user
	return d
}

func TestPushToAWorkspaceWritesAFileInsideTheHome(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := newLocalClient(t, home)

	if err := Push(context.Background(), c, workspaceFile(currentUser(t), "~/.config/robot/key.json"), "{}"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".config", "robot", "key.json")
	got, _ := os.ReadFile(path)
	if string(got) != "{}" {
		t.Fatalf("got %q", got)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestPushToAWorkspaceTightensAnExistingFile(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := newLocalClient(t, home)
	dir := filepath.Join(home, ".devmachine", "sentry")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "env"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Push(context.Background(), c, workspaceSecret(currentUser(t)), "v"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(dir, "env"))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	if got := sourceAndEcho(t, mustRead(t, filepath.Join(dir, "env")), "SENTRY_TOKEN"); got != "v" {
		t.Fatalf("got %q", got)
	}
}

func TestPushToAWorkspaceRefusesADirectoryThatLinksOutOfTheHome(t *testing.T) {
	home, outside := homeAndOutside(t)
	c := newLocalClient(t, home)
	if err := os.Symlink(outside, filepath.Join(home, ".config")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(outside)

	err := Push(context.Background(), c, workspaceFile(currentUser(t), "~/.config/robot/key.json"), "{}")
	if err == nil || !strings.Contains(err.Error(), "outside the workspace's home") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "robot")); !os.IsNotExist(err) {
		t.Fatalf("it created a directory outside the home: %v", err)
	}
	after, _ := os.Stat(outside)
	if after.Mode() != before.Mode() {
		t.Fatalf("it changed the mode outside the home: %v -> %v", before.Mode(), after.Mode())
	}
}

func TestPushToAWorkspaceRefusesAFileThatIsASymlink(t *testing.T) {
	home, outside := homeAndOutside(t)
	c := newLocalClient(t, home)
	victim := filepath.Join(outside, "shadow")
	if err := os.WriteFile(victim, []byte("root:hash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".devmachine", "sentry")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, "env")); err != nil {
		t.Fatal(err)
	}

	err := Push(context.Background(), c, workspaceSecret(currentUser(t)), "v")
	if err == nil || !strings.Contains(err.Error(), "outside the workspace's home") {
		t.Fatalf("got %v", err)
	}
	if got := mustRead(t, victim); got != "root:hash\n" {
		t.Fatalf("the linked file was written: %q", got)
	}
	info, _ := os.Stat(victim)
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("the linked file's mode changed: %v", info.Mode().Perm())
	}
}

func TestPushToAWorkspaceRefusesAPathThatClimbsOutOfTheHome(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := newLocalClient(t, home)

	err := Push(context.Background(), c, workspaceFile(currentUser(t), "~/../outside/key.json"), "{}")
	if err == nil || !strings.Contains(err.Error(), "outside the workspace's home") {
		t.Fatalf("got %v", err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
