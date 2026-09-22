package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoCommitIsSilentOutsideARepository(t *testing.T) {
	// Most people will never run `setup git`. It must cost them nothing.
	dir := t.TempDir()
	AutoCommit(context.Background(), dir, "chore(config): add machine box")

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		t.Fatal("it created a repository")
	}
}

func TestAutoCommitNeverFailsTheCommandThatCalledIt(t *testing.T) {
	// A workspace that exists on the machine and not in the history is a
	// nuisance. A `workspaces new` that fails after creating the user is worse.
	// AutoCommit returns nothing, so there is nothing for a caller to check —
	// the guarantee is that it never panics on a repository it cannot commit to.
	dir := initedRepo(t)
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, ".git"), 0o755) })

	AutoCommit(context.Background(), dir, "chore(config): add machine box")
}

func TestAutoCommitStillRefusesASecret(t *testing.T) {
	// The guard is in Commit, so this holds by construction. The test is here
	// because "by construction" is what stops being true.
	dir := initedRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "keys"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keys", "id_ed25519"), []byte("PRIVATE KEY\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	AutoCommit(context.Background(), dir, "chore(config): add machine box")

	body, err := os.ReadFile(filepath.Join(dir, "history.log"))
	if err != nil {
		t.Fatalf("the failure was not recorded: %v", err)
	}
	if !strings.Contains(string(body), "keys/id_ed25519") {
		t.Fatalf("the record does not name the file: %s", body)
	}

	// Tracked (`git ls-files`) reflects the index, which the guard leaves
	// staged on purpose. What matters is that no commit was ever made with it.
	out, err := exec.Command("git", "-C", dir, "log", "--all", "--name-only", "--format=").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "keys/id_ed25519") {
		t.Fatal("the key reached a commit")
	}
}

// initedRepo is a real, empty git repository, gpgsign off, so a test can
// commit into it without a signing key.
func initedRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"-C", dir, "init", "-q", "-b", "main"},
		{"-C", dir, "config", "commit.gpgsign", "false"},
		{"-C", dir, "config", "user.email", "test@example.com"},
		{"-C", dir, "config", "user.name", "test"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}
