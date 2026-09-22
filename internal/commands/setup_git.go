package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/repo"
	"github.com/spf13/cobra"
)

// runGh is the seam over the `gh` binary. Tests replace it; nothing else in
// this file calls it directly.
var runGh = func(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ghOnPath says whether `gh` is installed at all, before asking it anything.
var ghOnPath = func() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// ghAuthenticated asks `gh` itself, rather than guessing from a token file:
// the CLI does not know where `gh` keeps its session, and it should not learn.
func ghAuthenticated(ctx context.Context, dir string) bool {
	_, err := runGh(ctx, dir, "auth", "status")
	return err == nil
}

type setupGitOptions struct {
	yes   bool
	check bool
}

func newSetupGitCmd(opts *options) *cobra.Command {
	var s setupGitOptions

	c := &cobra.Command{
		Use:   "git",
		Short: "Make the configuration directory a git repository",
		Long: "Writes a `.gitignore` before it ever runs `git init`, so a private " +
			"key or a secret never has a window in which `git add -A` can pick it " +
			"up. Only config.yml and packages.lock are committed.\n\n" +
			"The remote this pushes to must be private: the configuration holds " +
			"real hostnames and usernames. With `gh` on `PATH` and authenticated " +
			"it offers to create one; otherwise it prints the two commands to run " +
			"by hand.\n\n" +
			"A directory that already tracks a key from before this command " +
			"existed is refused, not fixed: untracking a file does not remove it " +
			"from a commit that already has it, and that key must be rotated.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			return runSetupGit(cmd.Context(), dir, cmd.InOrStdin(), cmd.OutOrStdout(), s)
		},
	}
	c.Flags().BoolVar(&s.yes, "yes", false, "create the private remote and push without asking")
	c.Flags().BoolVar(&s.check, "check", false, "say what would happen, and write nothing")
	return c
}

// runSetupGit is the whole flow: write the ignore file, start the repository,
// commit what is safe, refuse what is not, then offer a remote.
//
// The order matters and is not reordered by convenience: .gitignore is
// written before `git init`, which is before anything is staged. Written
// after, there is a window where `git add -A` picks up a private key, and
// removing it from a history afterwards is work most people do badly.
func runSetupGit(ctx context.Context, dir string, in io.Reader, out io.Writer, opts setupGitOptions) error {
	if !repo.IsRepo(dir) {
		if opts.check {
			fmt.Fprintf(out, "would write %s and start a git repository in %s\n",
				filepath.Join(dir, ".gitignore"), dir)
			return nil
		}

		ignorePath := filepath.Join(dir, ".gitignore")
		if err := os.WriteFile(ignorePath, []byte(repo.GitIgnore()), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", ignorePath, err)
		}
		if err := repo.Init(ctx, dir); err != nil {
			return err
		}
		if err := repo.Commit(ctx, dir, "chore(config): start tracking this configuration"); err != nil {
			return err
		}
		fmt.Fprintf(out, "started a git repository in %s\n", dir)
	}

	tracked, err := repo.Tracked(ctx, dir)
	if err != nil {
		return err
	}
	if unsafe := repo.Unsafe(tracked); len(unsafe) > 0 {
		names := strings.Join(unsafe, " ")
		return fmt.Errorf(
			"this directory already tracks what must never be committed: %s\n"+
				"run `git rm --cached %s` to untrack it — that alone is not enough, because "+
				"a file already in a commit stays in that history: rotate it too",
			names, names)
	}

	fmt.Fprintf(out, "\nThe remote for %s must be private: it holds real hostnames and usernames.\n", dir)

	if opts.check {
		return nil
	}

	return ensureRemote(ctx, dir, in, out, opts.yes)
}

// ensureRemote offers a private remote when there is none yet. It never turns
// an existing remote into a push: that decision belongs to a person who typed
// `git push`, not to a passing `setup git`.
func ensureRemote(ctx context.Context, dir string, in io.Reader, out io.Writer, yes bool) error {
	remote, err := repo.HasRemote(ctx, dir)
	if err != nil {
		return err
	}
	if remote != "" {
		fmt.Fprintf(out, "origin is already %s\n", remote)
		return nil
	}

	if !ghOnPath() {
		printManualRemoteInstructions(out)
		return nil
	}
	if !ghAuthenticated(ctx, dir) {
		fmt.Fprintf(out, "\n`gh` is installed but not logged in. Run `gh auth login`, then:\n")
		printManualRemoteInstructions(out)
		return nil
	}

	if !yes {
		ok, err := confirm(in, out, "Create a private repository with `gh` and push this configuration to it?")
		if err != nil {
			return err
		}
		if !ok {
			printManualRemoteInstructions(out)
			return nil
		}
	}

	if _, err := runGh(ctx, dir, "repo", "create", "--private", "--source", ".", "--remote", "origin", "--push"); err != nil {
		return fmt.Errorf("gh repo create: %w", err)
	}
	fmt.Fprintf(out, "created a private repository and pushed to it\n")
	return nil
}

func printManualRemoteInstructions(out io.Writer) {
	fmt.Fprintf(out, "\nCreate a private repository yourself, then run:\n")
	fmt.Fprintf(out, "  git remote add origin <url-of-a-private-repository>\n")
	fmt.Fprintf(out, "  git -C . push -u origin main\n")
}
