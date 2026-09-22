package repo

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// run is the seam. Tests replace it; nothing else calls git.
var run = func(ctx context.Context, dir string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message != "" {
			return stdout.String(), fmt.Errorf("%w: %s", err, message)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// IsRepo says whether dir is already a git repository.
func IsRepo(dir string) bool {
	_, err := run(context.Background(), dir, "rev-parse", "--git-dir")
	return err == nil
}

// Init starts a repository with `main` as its first branch.
func Init(ctx context.Context, dir string) error {
	if _, err := run(ctx, dir, "init", "-q", "-b", "main"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	return nil
}

// Tracked lists every path git already follows in dir.
func Tracked(ctx context.Context, dir string) ([]string, error) {
	out, err := run(ctx, dir, "ls-files")
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	return splitLines(out), nil
}

// Commit stages everything, refuses if anything unsafe is staged, and commits
// signed. A message with more than one line is refused before git ever sees
// it: the operator's own rule, and this is the CLI writing the commit.
//
// An index that matches HEAD is not a failure — `sync` writing the same
// config.yml twice is the normal case — so `git commit` reporting nothing to
// commit is swallowed rather than returned.
func Commit(ctx context.Context, dir, message string) error {
	if strings.Contains(message, "\n") {
		return fmt.Errorf("commit message has more than one line: %q", message)
	}

	if _, err := run(ctx, dir, "add", "-A"); err != nil {
		return fmt.Errorf("git add -A: %w", err)
	}

	staged, err := run(ctx, dir, "diff", "--cached", "--name-only")
	if err != nil {
		return fmt.Errorf("git diff --cached --name-only: %w", err)
	}

	if unsafe := Unsafe(splitLines(staged)); len(unsafe) > 0 {
		// `git add -A` staged it before the guard looked. Leaving it there
		// protects this commit and arms the next one somebody types by hand.
		if _, err := run(ctx, dir, append([]string{"reset", "--"}, unsafe...)...); err != nil {
			return fmt.Errorf("unstaging %s: %w", strings.Join(unsafe, " "), err)
		}
		names := strings.Join(unsafe, " ")
		return fmt.Errorf(
			"refusing to commit %s: it must never reach this history\n"+
				"run `git rm --cached %s` — untracking does not remove it from a commit "+
				"that already has it, so rotate it too",
			names, names)
	}

	// No -S. Whether a commit is signed is the operator's own git
	// configuration, and forcing it here breaks every machine that has no
	// signing key — which is most of them, including CI — while overriding a
	// setting that was never ours to override.
	if _, err := run(ctx, dir, "commit", "-m", message); err != nil {
		if strings.Contains(err.Error(), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

// HasRemote returns the URL of the `origin` remote, or "" when there is none.
func HasRemote(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		if strings.Contains(err.Error(), "No such remote") {
			return "", nil
		}
		return "", fmt.Errorf("git remote get-url origin: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// AddRemote points `origin` at url.
func AddRemote(ctx context.Context, dir, url string) error {
	if _, err := run(ctx, dir, "remote", "add", "origin", url); err != nil {
		return fmt.Errorf("git remote add origin: %w", err)
	}
	return nil
}

// Push pushes the current branch to `origin`, and refuses without one: a push
// nobody asked for is worse than a push that has to be asked for twice.
func Push(ctx context.Context, dir string) error {
	remote, err := HasRemote(ctx, dir)
	if err != nil {
		return err
	}
	if remote == "" {
		return fmt.Errorf("there is no remote named origin: add one before pushing")
	}
	if _, err := run(ctx, dir, "push", "-u", "origin", "main"); err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}

func splitLines(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
