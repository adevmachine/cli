package repo

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/adevmachine/cli/internal/history"
)

// AutoCommit commits a change the CLI just made, when dir is already a git
// repository (made by `devmachine setup git`). It is silent outside one: most
// configurations never become a repository, and this must cost them nothing.
//
// It never fails the command that called it. A workspace that exists on the
// machine and not in the history is a nuisance; `workspaces new` failing after
// the account already exists on the machine is worse. A failure is recorded to
// history.log instead, which already exists to say what ran and whether it
// worked.
func AutoCommit(ctx context.Context, dir, message string) {
	AutoCommitTo(ctx, dir, message, os.Stderr)
}

// AutoCommitTo is AutoCommit with somewhere to complain to.
//
// A refusal is not a quiet condition: it means the directory holds something
// that must never be committed, and history.log is not where anybody looks.
func AutoCommitTo(ctx context.Context, dir, message string, warn io.Writer) {
	if !IsRepo(dir) {
		return
	}
	if err := Commit(ctx, dir, message); err != nil {
		fmt.Fprintf(warn, "the configuration was not committed: %v\n", err)
		history.Append(dir, history.Entry{
			At:      time.Now(),
			Target:  "git",
			Command: "auto-commit failed: " + message + ": " + err.Error(),
			OK:      false,
		})
	}
}
