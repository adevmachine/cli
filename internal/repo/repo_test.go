package repo

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// fakeRepo stands in for git: it never shells out, so a test proves what the
// package decided rather than what a real git binary happened to do.
type fakeRepo struct {
	dir     string
	calls   [][]string
	outputs map[string]string
	errs    map[string]error
}

// fakeGit installs the fake in place of the seam for the life of the test.
// outputs is keyed by the git subcommand and its arguments joined with a
// space, e.g. "diff --cached --name-only".
func fakeGit(t *testing.T, outputs map[string]string) *fakeRepo {
	t.Helper()
	g := &fakeRepo{dir: t.TempDir(), outputs: outputs, errs: map[string]error{}}

	old := run
	run = g.run
	t.Cleanup(func() { run = old })

	return g
}

func (g *fakeRepo) run(_ context.Context, _ string, args ...string) (string, error) {
	g.calls = append(g.calls, args)
	if err := g.errs[args[0]]; err != nil {
		return "", err
	}
	if out, ok := g.outputs[strings.Join(args, " ")]; ok {
		return out, nil
	}
	return "", nil
}

// lastCall returns the arguments of the most recent call to the named git
// subcommand, or nil when it was never called.
func (g *fakeRepo) lastCall(name string) []string {
	for i := len(g.calls) - 1; i >= 0; i-- {
		if g.calls[i][0] == name {
			return g.calls[i]
		}
	}
	return nil
}

func TestCommitRefusesWhenSomethingUnsafeIsStaged(t *testing.T) {
	// The guard refuses; it does not warn. A warning printed above a
	// successful commit is a warning nobody reads.
	git := fakeGit(t, map[string]string{
		"diff --cached --name-only": "config.yml\nkeys/id_ed25519\n",
	})

	err := Commit(context.Background(), git.dir, "chore: something")
	if err == nil {
		t.Fatal("it committed a private key")
	}
	if !strings.Contains(err.Error(), "keys/id_ed25519") {
		t.Fatalf("the error does not name the file: %v", err)
	}
	if slices.ContainsFunc(git.calls, func(c []string) bool { return c[0] == "commit" }) {
		t.Fatal("it reached git commit")
	}
}

func TestCommitSignsAndStaysOnOneLine(t *testing.T) {
	// The operator's own rule, and the CLI is writing these commits.
	git := fakeGit(t, nil)
	if err := Commit(context.Background(), git.dir, "chore(config): add machine"); err != nil {
		t.Fatal(err)
	}

	args := git.lastCall("commit")
	if args == nil {
		t.Fatal("it never reached git commit")
	}
	if !slices.Contains(args, "-S") {
		t.Fatalf("the commit is not signed: %#v", args)
	}
	for _, a := range args {
		if strings.Contains(a, "\n") {
			t.Fatalf("the message has more than one line: %q", a)
		}
	}
}

func TestCommitSaysNothingWhenThereIsNothingToCommit(t *testing.T) {
	// `git commit` with an empty index exits 1. That is not a failure here —
	// `sync` writing the same config.yml twice is the normal case.
	git := fakeGit(t, nil)
	git.errs["commit"] = errors.New("exit status 1: nothing to commit, working tree clean")

	if err := Commit(context.Background(), git.dir, "chore(config): add machine"); err != nil {
		t.Fatalf("an empty commit failed the caller: %v", err)
	}
}

func TestCommitRejectsAMultiLineMessage(t *testing.T) {
	git := fakeGit(t, nil)
	if err := Commit(context.Background(), git.dir, "chore: one\nchore: two"); err == nil {
		t.Fatal("it accepted a multi-line message")
	}
	if git.lastCall("commit") != nil {
		t.Fatal("it reached git commit with a bad message")
	}
}

func TestPushRefusesWithoutARemote(t *testing.T) {
	git := fakeGit(t, nil)
	git.errs["remote"] = errors.New("exit status 2: No such remote 'origin'")

	err := Push(context.Background(), git.dir)
	if err == nil {
		t.Fatal("it pushed with no remote configured")
	}
	if git.lastCall("push") != nil {
		t.Fatal("it reached git push")
	}
}

func TestPushPushesToOriginMain(t *testing.T) {
	git := fakeGit(t, map[string]string{
		"remote get-url origin": "git@github.com:alice/config.git\n",
	})

	if err := Push(context.Background(), git.dir); err != nil {
		t.Fatal(err)
	}
	args := git.lastCall("push")
	if args == nil || !slices.Contains(args, "origin") {
		t.Fatalf("it did not push to origin: %#v", args)
	}
}

func TestTrackedListsWhatGitFollows(t *testing.T) {
	git := fakeGit(t, map[string]string{
		"ls-files": "config.yml\npackages.lock\n",
	})

	got, err := Tracked(context.Background(), git.dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"config.yml", "packages.lock"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestCommitUnstagesWhatItRefused(t *testing.T) {
	// `git add -A` staged it before the guard looked. Refusing the commit and
	// leaving it staged protects this commit and arms the operator's next
	// one: they type `git commit`, and the key goes in.
	git := fakeGit(t, map[string]string{
		"diff --cached --name-only": "config.yml\nkeys/id_ed25519\n",
	})

	if err := Commit(t.Context(), git.dir, "chore: something"); err == nil {
		t.Fatal("it committed a private key")
	}

	if git.lastCall("reset") == nil {
		t.Fatalf("nothing was unstaged: %#v", git.calls)
	}
	if !slices.Contains(git.lastCall("reset"), "keys/id_ed25519") {
		t.Fatalf("the key is still staged: %#v", git.lastCall("reset"))
	}
}
