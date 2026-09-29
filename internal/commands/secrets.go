package commands

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/credentials"
	"github.com/mydevmachine/devmachine/internal/packages"
	"github.com/mydevmachine/devmachine/internal/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// readSecret is a seam so a test can supply a value without a terminal.
var readSecret = promptForSecret

func newSecretsCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Tokens the CLI needs, kept in the OS keychain",
	}
	cmd.AddCommand(newSecretsSetCmd(opts), newSecretsListCmd(opts), newSecretsRmCmd(opts),
		newSecretsExampleCmd(opts))
	return cmd
}

func newSecretsExampleCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "example",
		Short: "List the secret values a machine's packages need, with no values",
		Long: "One `<NAME>=` line per secret, in the shape `secrets set` expects the " +
			"name in. It never reads a stored value, so it prints one whether or " +
			"not anything has been set yet.\n\n" +
			"It writes to standard output, on purpose, and never to a file: a file " +
			"named `.env.example` sits one typo away from `.env`, in a directory " +
			"that may be a git repository, and that is not a trap this command sets.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := credentialsOnMachine(cmd.Context(), opts)
			if err != nil {
				return err
			}

			seen := map[string]bool{}
			for _, c := range d.wanted {
				if c.Kind != packages.KindSecret || c.Env == "" || seen[c.Env] {
					continue
				}
				seen[c.Env] = true
				cmd.Printf("%s=\n", c.Env)
			}
			return nil
		},
	}
}

func secretsDir(opts *options) (string, error) {
	dir, _, err := config.Dir(opts.configDir)
	return dir, err
}

func newSecretsSetCmd(opts *options) *cobra.Command {
	var fromStdin, push bool
	var workspace, envFile string

	c := &cobra.Command{
		Use:   "set <name> [value]",
		Short: "Store a secret",
		Long: "With no value the secret is read from the terminal without " +
			"echoing it, so it never reaches the shell history.\n\n" +
			"With --workspace, this is one of that workspace's own app " +
			"secrets, not a value a package declared: it is delivered on " +
			"the next `devmachine credentials push`, by default into " +
			"~/.devmachine/env, the sourceable file the workspace's shell " +
			"loads. --env-file delivers it into a dotenv file inside the " +
			"workspace instead, editing that file in place — see " +
			"docs/concepts/credentials.md for what that means.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := secretsDir(opts)
			if err != nil {
				return err
			}

			name := args[0]
			var value string
			switch {
			case len(args) == 2:
				value = args[1]
			case fromStdin:
				line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if err != nil && line == "" {
					return fmt.Errorf("reading the value: %w", err)
				}
				value = strings.TrimRight(line, "\r\n")
			default:
				value, err = readSecret(cmd.ErrOrStderr(), name)
				if err != nil {
					return err
				}
			}

			if value == "" {
				return fmt.Errorf("the value is empty: pass it as an argument, with --stdin, or type it when asked")
			}
			if envFile != "" && workspace == "" {
				return fmt.Errorf("--env-file needs --workspace")
			}
			if push && workspace == "" {
				return fmt.Errorf("--push needs --workspace")
			}

			key := name
			var t secrets.Target
			if workspace != "" {
				rel := ""
				if envFile != "" {
					rel, err = credentials.SafeWorkspacePath(envFile)
					if err != nil {
						return err
					}
				}
				key = workspace + "/" + name
				t = secrets.Target{Workspace: workspace, Name: name, EnvFile: rel}
			}

			if err := secrets.Set(dir, key, value); err != nil {
				return err
			}
			if workspace != "" {
				if err := secrets.SetTarget(dir, t); err != nil {
					return err
				}
			}
			cmd.Printf("stored %s\n", key)

			if push {
				destination, err := pushWorkspaceSecret(cmd.Context(), opts, t, value)
				if err != nil {
					return err
				}
				cmd.Printf("delivered %s -> %s\n", key, destination)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&fromStdin, "stdin", false, "read the value from standard input")
	c.Flags().StringVar(&workspace, "workspace", "",
		"this is one of that workspace's own app secrets, stored under <workspace>/<name>")
	c.Flags().StringVar(&envFile, "env-file", "",
		"deliver into this dotenv file instead, relative to the workspace's home")
	c.Flags().BoolVar(&push, "push", false, "also deliver it now, instead of waiting for the next `credentials push`")
	return c
}

// pushWorkspaceSecret delivers one workspace secret's value right away, for
// `secrets set --push`. It dials only the machine that one workspace lives
// on — the same round trip `credentials push` would spend on it later.
func pushWorkspaceSecret(ctx context.Context, opts *options, t secrets.Target, value string) (string, error) {
	tgt, err := workspaceTarget(opts, t.Workspace)
	if err != nil {
		return "", err
	}
	client, _, err := dial(ctx, tgt.machine, tgt.user)
	if err != nil {
		return "", err
	}
	defer client.Close()

	rel := t.EnvFile
	if rel == "" {
		rel = credentials.DefaultEnvFile
	}
	if err := credentials.PushWorkspaceEnv(ctx, client, tgt.user, rel, t.Name, value, t.EnvFile != ""); err != nil {
		return "", err
	}
	return rel, nil
}

// secretRow is one line of `secrets list`: a name, and where it goes when it
// is a workspace's own secret. A value is never in it.
type secretRow struct {
	Name      string `json:"name"`
	Workspace string `json:"workspace,omitempty"`
	Target    string `json:"target,omitempty"`
}

func newSecretsListCmd(opts *options) *cobra.Command {
	var workspace string

	c := &cobra.Command{
		Use:   "list",
		Short: "List the stored secrets by name",
		Long: "Names and, for a workspace's own secret, where it is delivered. " +
			"A value is never printed by this command.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := secretsDir(opts)
			if err != nil {
				return err
			}
			names, err := secrets.List(dir)
			if err != nil {
				return err
			}
			targets, err := secrets.Targets(dir)
			if err != nil {
				return err
			}
			byKey := make(map[string]secrets.Target, len(targets))
			for _, t := range targets {
				byKey[t.Key()] = t
			}

			var rows []secretRow
			for _, n := range names {
				ws, name := splitSecretKey(n)
				if workspace != "" && ws != workspace {
					continue
				}
				row := secretRow{Name: name, Workspace: ws}
				if t, ok := byKey[n]; ok {
					row.Target = workspaceSecretTargetPath(t)
				}
				rows = append(rows, row)
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), struct {
					Secrets []string    `json:"secrets"`
					Targets []secretRow `json:"targets,omitempty"`
				}{names, rows})
			}
			for _, r := range rows {
				line := r.Name
				if r.Workspace != "" {
					line = r.Workspace + "/" + r.Name
				}
				if r.Target != "" {
					line += " -> " + r.Target
				}
				cmd.Println(line)
			}
			return nil
		},
	}
	c.Flags().StringVar(&workspace, "workspace", "", "list only this workspace's own secrets")
	return c
}

// splitSecretKey pulls a workspace secret's key apart. A bare name — no
// slash — is not a workspace secret at all, and ws comes back empty.
func splitSecretKey(key string) (workspace, name string) {
	if ws, n, ok := strings.Cut(key, "/"); ok {
		return ws, n
	}
	return "", key
}

func newSecretsRmCmd(opts *options) *cobra.Command {
	var workspace string
	var fromFile bool

	c := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a secret",
		Long: "With --workspace and --from-file, the name is also removed " +
			"from its delivery file on the next `devmachine credentials push` " +
			"— it is not edited here, so this never needs to reach the machine.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := secretsDir(opts)
			if err != nil {
				return err
			}
			name := args[0]
			key := name
			if workspace != "" {
				key = workspace + "/" + name
			}
			if fromFile && workspace == "" {
				return fmt.Errorf("--from-file needs --workspace")
			}

			if workspace != "" {
				t, ok, err := secrets.FindTarget(dir, workspace, name)
				if err != nil {
					return err
				}
				switch {
				case ok && fromFile:
					t.PendingRemoval = true
					if err := secrets.SetTarget(dir, t); err != nil {
						return err
					}
				case ok:
					if err := secrets.RemoveTarget(dir, workspace, name); err != nil {
						return err
					}
				case fromFile:
					return fmt.Errorf("no delivery target recorded for %s: nothing to remove from a file", key)
				}
			}

			if err := secrets.Delete(dir, key); err != nil {
				return err
			}
			cmd.Printf("removed %s\n", key)
			return nil
		},
	}
	c.Flags().StringVar(&workspace, "workspace", "", "the workspace this secret belongs to")
	c.Flags().BoolVar(&fromFile, "from-file", false,
		"also remove it from its delivery file, on the next `credentials push`")
	return c
}

func promptForSecret(w io.Writer, name string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("no terminal to ask on: pass the value as an argument or use --stdin")
	}
	fmt.Fprintf(w, "value for %s: ", name)
	body, err := term.ReadPassword(fd)
	fmt.Fprintln(w)
	if err != nil {
		return "", fmt.Errorf("reading the value: %w", err)
	}
	return string(body), nil
}
