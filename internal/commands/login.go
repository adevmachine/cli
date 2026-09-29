package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/credentials"
	"github.com/mydevmachine/devmachine/internal/packages"
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
	if d.Name == tailscalePackage {
		return finishTailscaleLogin(cmd, opts, found.machine)
	}
	return keepForTheWorkspaces(cmd, found.machine, d)
}

// tailscaleStatusCommand asks a machine's own tailscale for its status. It is
// the same shape `tailscale status --json` gives everywhere: this is how the
// CLI learns the name the machine answers to on the tailnet, which is what a
// `tailscale:<name>` host entry has to match.
const tailscaleStatusCommand = "tailscale status --json"

// finishTailscaleLogin is what makes `devmachine login tailscale` more than a
// login: once the machine has signed in, it can say what it is called on the
// tailnet, and that name is what turns into a private address in the
// configuration — automatically, because asking somebody to copy a hostname
// out of JSON is not a step anyone should have to do by hand.
func finishTailscaleLogin(cmd *cobra.Command, opts *options, m config.Machine) error {
	dir, _, err := config.Dir(opts.configDir)
	if err != nil {
		return err
	}

	client, _, err := dial(cmd.Context(), m, "")
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	byHand := fmt.Sprintf(
		"logged in. Add this line above the public address, under %s in config.yml, to reach it that way too:\n  tailscale:<name>\n",
		m.Name)

	out, err := client.Run(cmd.Context(), tailscaleStatusCommand)
	if err != nil {
		cmd.Printf("logged in, but could not read its tailnet name (%v).\n", err)
		cmd.Print(byHand)
		return nil
	}
	name, err := tailscaleSelfHostName(out)
	if err != nil {
		cmd.Printf("logged in, but could not read its tailnet name from `%s` (%v).\n", tailscaleStatusCommand, err)
		cmd.Print(byHand)
		return nil
	}

	entry := "tailscale:" + name
	for _, h := range m.Hosts {
		if h.Address == entry {
			cmd.Printf("logged in; %s is already in %s's hosts.\n", entry, m.Name)
			return nil
		}
	}

	addresses := make([]string, 0, len(m.Hosts)+1)
	addresses = append(addresses, entry)
	for _, h := range m.Hosts {
		addresses = append(addresses, h.Address)
	}
	if err := config.SetMachineHosts(dir, m.Name, addresses); err != nil {
		return err
	}
	cmd.Printf("logged in; added %s above the public address for %s. The public address stays as a fallback.\n",
		entry, m.Name)
	return nil
}

// tailscaleSelfHostName reads the name the machine answers to on the
// tailnet out of its own `tailscale status --json`.
func tailscaleSelfHostName(body string) (string, error) {
	var status struct {
		Self struct {
			HostName string `json:"HostName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		return "", fmt.Errorf("parsing the tailscale status: %w", err)
	}
	if status.Self.HostName == "" {
		return "", errors.New("the tailscale status did not name the machine")
	}
	return status.Self.HostName, nil
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
