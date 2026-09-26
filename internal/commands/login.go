package commands

import (
	"fmt"
	"slices"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/credentials"
	"github.com/adevmachine/cli/internal/packages"
	"github.com/spf13/cobra"
)

func newLoginCmd(opts *options) *cobra.Command {
	var workspace string

	c := &cobra.Command{
		Use:   "login <credential>",
		Short: "Sit through a login, in the place it has to happen",
		Long: "Nobody automates a browser login, and this does not pretend " +
			"to. It opens a real terminal where the credential belongs — the " +
			"machine, or one workspace — and runs the command the package " +
			"declared.\n\n" +
			"A machine credential is logged into once, and kept in " +
			"/etc/devmachine/<name>/ so the next sync copies it into every " +
			"workspace that asks for it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd, opts, args[0], workspace)
		},
	}
	c.Flags().StringVar(&workspace, "workspace", "", "log in inside this workspace")
	return c
}

func runLogin(cmd *cobra.Command, opts *options, name, workspace string) error {
	found, err := credentialsOnMachine(cmd.Context(), opts)
	if err != nil {
		return err
	}
	d, err := pickCredential(found.wanted, name, workspace)
	if err != nil {
		return err
	}

	tgt := target{machine: found.machine, user: d.LinuxUser, workspace: d.Workspace}
	address, err := firstAddress(found.machine, "login")
	if err != nil {
		return err
	}
	if _, err := lookPath("ssh"); err != nil {
		return fmt.Errorf("ssh is not installed on this machine")
	}
	if err := verifySystemHost(cmd.Context(), found.machine); err != nil {
		return err
	}

	cmd.Printf("opening a session on %s to run: %s\n", found.machine.Name, d.Command)
	runErr := runInteractive("ssh", loginArgs(found.machine, tgt.login(), address, d.Command)...)
	record(opts, tgt, "login "+credentials.Key(d), runErr == nil)
	if runErr != nil {
		return fmt.Errorf("the login did not finish: %w", runErr)
	}

	if d.Workspace != "" {
		cmd.Printf("logged in as %s\n", tgt.login())
		return nil
	}
	return keepForTheWorkspaces(cmd, found.machine, d)
}

// loginArgs is the session the login runs in.
//
// `-t` is the whole point: without a terminal a device code prints into a pipe
// nobody is reading, and a prompt waits for an answer that can never come.
func loginArgs(m config.Machine, user, address, command string) []string {
	args := append([]string{"-t"}, strictSSHArgs(m)...)
	return append(args, user+"@"+address, command)
}

// pickCredential finds the one credential this command acts on, and refuses to
// guess when a name belongs to more than one place.
func pickCredential(wanted []credentials.Declared, name, workspace string) (credentials.Declared, error) {
	var matches []credentials.Declared
	for _, d := range wanted {
		if d.Name == name {
			matches = append(matches, d)
		}
	}
	if len(matches) == 0 {
		return credentials.Declared{}, fmt.Errorf(
			"no package installed here declares a credential named %q: `devmachine credentials list` shows the ones that do",
			name)
	}
	if kind := matches[0].Kind; kind != packages.KindManual {
		return credentials.Declared{}, fmt.Errorf(
			"credential %q is a %s, and nobody logs into a %s: store it with `devmachine secrets set %s`, "+
				"then deliver it with `devmachine credentials push`",
			name, kind, kind, name)
	}

	machineScoped := matches[0].Workspace == ""
	if workspace == "" {
		if machineScoped {
			return matches[0], nil
		}
		return credentials.Declared{}, fmt.Errorf(
			"credential %q belongs to a workspace, and each one logs in for itself: name it with --workspace (%s)",
			name, strings.Join(workspacesOf(matches), ", "))
	}
	if machineScoped {
		return credentials.Declared{}, fmt.Errorf(
			"credential %q belongs to the machine, not to a workspace: run `devmachine login %s` with no --workspace",
			name, name)
	}
	for _, d := range matches {
		if d.Workspace == workspace {
			return d, nil
		}
	}
	return credentials.Declared{}, fmt.Errorf(
		"workspace %q does not ask for credential %q: the workspaces that do are %s",
		workspace, name, strings.Join(workspacesOf(matches), ", "))
}

func workspacesOf(matches []credentials.Declared) []string {
	var out []string
	for _, d := range matches {
		out = append(out, d.Workspace)
	}
	slices.Sort(out)
	return out
}

// keepScript copies what the login left into /etc/devmachine/<name>/.
//
// The tool wrote wherever it writes, and it is not going to learn a new place.
// The copy is what makes one login on the machine reachable by every workspace
// that declares the package, and it is the directory `sync` distributes from.
const keepScript = `set -eu
umask 077
from=%s
dir=%s
case "$from" in
'~/'*) from="$HOME/${from#'~/'}" ;;
esac
if [ ! -e "$from" ]; then
	echo "the login left nothing at $from" >&2
	exit 1
fi
mkdir -p "$dir"
chmod 0700 "$dir"
cp -R "$from" "$dir/"
chown -R root "$dir"
chmod -R go-rwx "$dir"
printf '%%s\n' "$dir"
`

func keepForTheWorkspaces(cmd *cobra.Command, machine config.Machine, d credentials.Declared) error {
	dir := credentials.MachineDir(d.Name)
	if d.StoredAt == "" {
		return fmt.Errorf(
			"package %q did not say where %s stores the result, so there is nothing to copy into %s: "+
				"add `stored_at` to its declaration",
			d.Package, d.Name, dir)
	}

	client, _, err := dial(cmd.Context(), machine, "")
	if err != nil {
		return err
	}
	defer client.Close()

	script := fmt.Sprintf(keepScript, quoteForShell(d.StoredAt), quoteForShell(dir))
	if _, err := client.Run(cmd.Context(), script); err != nil {
		return fmt.Errorf("keeping the result in %s: %w", dir, err)
	}
	cmd.Printf("logged in, and kept in %s\n", dir)
	cmd.Println("run `devmachine sync` to copy it into the workspaces that ask for it")
	return nil
}

// quoteForShell makes a value safe to write into a script.
func quoteForShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
