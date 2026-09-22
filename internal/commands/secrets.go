package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/secrets"
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
	var fromStdin bool

	c := &cobra.Command{
		Use:   "set <name> [value]",
		Short: "Store a secret",
		Long: "With no value the secret is read from the terminal without " +
			"echoing it, so it never reaches the shell history.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := secretsDir(opts)
			if err != nil {
				return err
			}

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
				value, err = readSecret(cmd.ErrOrStderr(), args[0])
				if err != nil {
					return err
				}
			}

			if value == "" {
				return fmt.Errorf("the value is empty: pass it as an argument, with --stdin, or type it when asked")
			}
			if err := secrets.Set(dir, args[0], value); err != nil {
				return err
			}
			cmd.Printf("stored %s\n", args[0])
			return nil
		},
	}
	c.Flags().BoolVar(&fromStdin, "stdin", false, "read the value from standard input")
	return c
}

func newSecretsListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the stored secrets by name",
		Long:  "Names only. A value is never printed by this command.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := secretsDir(opts)
			if err != nil {
				return err
			}
			names, err := secrets.List(dir)
			if err != nil {
				return err
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), struct {
					Secrets []string `json:"secrets"`
				}{names})
			}
			for _, n := range names {
				cmd.Println(n)
			}
			return nil
		},
	}
}

func newSecretsRmCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := secretsDir(opts)
			if err != nil {
				return err
			}
			if err := secrets.Delete(dir, args[0]); err != nil {
				return err
			}
			cmd.Printf("removed %s\n", args[0])
			return nil
		},
	}
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
