package commands

import (
	"context"
	"fmt"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/credentials"
	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/secrets"
	"github.com/spf13/cobra"
)

// The three answers a row can give about one credential.
const (
	statusPresent = "present"
	statusMissing = "missing"
	// statusUnknown is a credential whose package never said where its tool
	// keeps the result. "I cannot tell" is not "it is not there".
	statusUnknown = "unknown"
)

// credentialJSON is one row of the report, and it never carries a value.
type credentialJSON struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Package   string `json:"package"`
	Workspace string `json:"workspace,omitempty"`
	Place     string `json:"place,omitempty"`
	Status    string `json:"status"`
	Fix       string `json:"fix,omitempty"`
}

func newCredentialsCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credentials",
		Short: "What the packages on a machine cannot work without",
		Long: "A package declares what its tool needs and how each one is " +
			"obtained. This is where you see what is missing, and what " +
			"delivers it.",
	}
	cmd.AddCommand(newCredentialsListCmd(opts))
	return cmd
}

// declared is the machine, its credentials, and the configuration directory
// they are reported against.
type declared struct {
	dir     string
	machine config.Machine
	wanted  []credentials.Declared
}

// credentialsOnMachine reads the configuration and the packages, and returns
// every credential the machine and its workspaces declare.
func credentialsOnMachine(ctx context.Context, opts *options) (declared, error) {
	dir, _, err := config.Dir(opts.configDir)
	if err != nil {
		return declared{}, err
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		return declared{}, err
	}
	machine, err := cfg.Machine(opts.machine)
	if err != nil {
		return declared{}, err
	}
	store, err := openStore(ctx, dir, cfg.Packages)
	if err != nil {
		return declared{}, err
	}
	plan, err := packages.ResolveMachine(store, cfg, machine, version)
	if err != nil {
		return declared{}, err
	}
	return declared{dir: dir, machine: machine, wanted: credentials.Wanted(plan)}, nil
}

// storedNames is the set of secrets the operator has already stored. Names
// only: a value is never read to draw a report.
func storedNames(dir string) (map[string]bool, error) {
	names, err := secrets.List(dir)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set, nil
}

// secretName is the secret a credential's value is read from.
//
// A workspace may need its own value for a credential every workspace
// declares, so `<workspace>/<name>` wins when it exists. Everything else falls
// back to the credential's name, which is what one value for the lot means.
func secretName(d credentials.Declared, stored map[string]bool) string {
	if key := credentials.Key(d); stored[key] {
		return key
	}
	return d.Name
}

// fixFor is the command that makes a missing credential arrive.
//
// It is the whole value of the report: nobody should have to work out what to
// do next from the fact that something is missing.
func fixFor(d credentials.Declared, stored map[string]bool) string {
	if d.Kind == packages.KindLogin {
		return credentials.LoginCommand(d)
	}
	if stored[secretName(d, stored)] {
		return "devmachine credentials push"
	}
	return fmt.Sprintf("devmachine secrets set %s, then devmachine credentials push", secretName(d, stored))
}

// statusOf reads one answer out of what the machine reported.
func statusOf(d credentials.Declared, present map[string]bool) string {
	if credentials.Place(d) == "" {
		return statusUnknown
	}
	if present[credentials.Key(d)] {
		return statusPresent
	}
	return statusMissing
}

func newCredentialsListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "What every package declares, and what is missing",
		Long: "Each row says what it is, whether the machine has it, and the " +
			"command that delivers it.\n\n" +
			"A row reads `unknown` when the package never said where its tool " +
			"keeps the result, so there is nowhere to look.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			found, err := credentialsOnMachine(cmd.Context(), opts)
			if err != nil {
				return err
			}
			stored, err := storedNames(found.dir)
			if err != nil {
				return err
			}

			present := map[string]bool{}
			if len(found.wanted) > 0 {
				client, _, err := dial(cmd.Context(), found.machine, "")
				if err != nil {
					return err
				}
				defer client.Close()

				present, err = credentials.Present(cmd.Context(), client, found.wanted)
				if err != nil {
					return err
				}
			}

			rows := make([]credentialJSON, 0, len(found.wanted))
			for _, d := range found.wanted {
				row := credentialJSON{
					Key:       credentials.Key(d),
					Name:      d.Name,
					Kind:      d.Kind,
					Package:   d.Package,
					Workspace: d.Workspace,
					Place:     credentials.Place(d),
					Status:    statusOf(d, present),
				}
				if row.Status != statusPresent {
					row.Fix = fixFor(d, stored)
				}
				rows = append(rows, row)
			}
			return reportCredentials(cmd, opts, rows)
		},
	}
}

func reportCredentials(cmd *cobra.Command, opts *options, rows []credentialJSON) error {
	if opts.format == formatJSON {
		return writeJSON(cmd.OutOrStdout(), struct {
			Credentials []credentialJSON `json:"credentials"`
		}{rows})
	}
	if len(rows) == 0 {
		cmd.Println("no credentials: nothing installed here declares one")
		return nil
	}

	width := 10
	for _, r := range rows {
		width = max(width, len(r.Key))
	}
	cmd.Printf("%-*s  %-7s  %-8s  %s\n", width, "CREDENTIAL", "KIND", "STATUS", "WHAT TO DO")
	for _, r := range rows {
		what := r.Fix
		if what == "" {
			what = "nothing: it is already there"
		}
		cmd.Printf("%-*s  %-7s  %-8s  %s\n", width, r.Key, r.Kind, r.Status, what)
	}
	return nil
}
