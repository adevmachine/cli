package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/term"

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
	fresh := !repo.IsRepo(dir)
	if opts.check {
		if fresh {
			fmt.Fprintf(out, "would write %s and start a git repository in %s\n",
				filepath.Join(dir, ".gitignore"), dir)
		} else {
			fmt.Fprintf(out, "would make sure %s covers every path that must never be committed\n",
				filepath.Join(dir, ".gitignore"))
		}
		return nil
	}

	// Before `git init`, and before anything is staged — but also on a
	// directory that has been a repository for months, which is the case the
	// operator's own configuration is in. Skipping it there leaves the CLI
	// writing keys/ and secrets.json into a tree that tracks everything.
	if err := ensureIgnoreFile(dir); err != nil {
		return err
	}

	if fresh {
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

	return ensureRemote(ctx, dir, in, out)
}

// ensureRemote offers a private remote when there is none yet. It never turns
// an existing remote into a push: that decision belongs to a person who typed
// `git push`, not to a passing `setup git`.
func ensureRemote(ctx context.Context, dir string, in io.Reader, out io.Writer) error {
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

	// Always asks, `--yes` or not. That flag means "do not ask me about the
	// writes in this directory"; it has never meant "publish". Creating a
	// repository on somebody's account is not the same class of action as
	// writing a local file, and one flag covering both is how a repository
	// gets created by a script nobody was watching.
	//
	// Where there is no terminal there is nobody to answer, and a question
	// asked there waits for an answer that never comes — `setup git --yes`
	// in a script hung exactly like that. The commands are printed instead.
	if !fromATerminal(in) {
		printManualRemoteInstructions(out)
		return nil
	}

	ok, err := confirm(in, out, "Create a private repository with `gh` and push this configuration to it?")
	if err != nil || !ok {
		printManualRemoteInstructions(out)
		return nil //nolint:nilerr // no answer is a no, not a failure
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

// ensureIgnoreFile makes sure .gitignore covers every path that must never be
// committed, adding only what is missing: a rule somebody else wrote is not
// ours to drop.
func ensureIgnoreFile(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	body, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(repo.GitIgnore()), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		return nil
	}

	have := strings.Split(string(body), "\n")
	var missing []string
	for _, rule := range repo.Ignored() {
		if !slices.Contains(have, rule) {
			missing = append(missing, rule)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	out := strings.TrimRight(string(body), "\n") + "\n\n" + repo.IgnoreHeading +
		strings.Join(missing, "\n") + "\n"
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// fromATerminal says whether somebody is there to answer a question. It is a
// variable so a test can stand in for a terminal it cannot have.
var fromATerminal = func(in io.Reader) bool {
	f, ok := in.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
