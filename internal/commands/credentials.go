package commands

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/credentials"
	"github.com/mydevmachine/devmachine/internal/packages"
	"github.com/mydevmachine/devmachine/internal/secrets"
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
	cmd.AddCommand(newCredentialsListCmd(opts), newCredentialsPushCmd(opts))
	return cmd
}

// declared is the machine, its credentials, and the configuration directory
// they are reported against.
type declared struct {
	dir     string
	cfg     config.Config
	machine config.Machine
	wanted  []credentials.Declared
	plan    packages.MachinePlan
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
	return declared{dir: dir, cfg: cfg, machine: machine, wanted: credentials.Wanted(plan), plan: plan}, nil
}

// workspaceTargetsOnMachine is every workspace secret target whose workspace
// lives on machine — the ones this machine's push has anything to say about.
func workspaceTargetsOnMachine(dir string, cfg config.Config, machine string) ([]secrets.Target, error) {
	all, err := secrets.Targets(dir)
	if err != nil {
		return nil, err
	}
	on := map[string]bool{}
	for _, w := range cfg.WorkspacesOn(machine) {
		on[w.Name] = true
	}
	var out []secrets.Target
	for _, t := range all {
		if on[t.Workspace] {
			out = append(out, t)
		}
	}
	return out, nil
}

// linuxUsersOn maps every workspace on machine to the account it owns.
func linuxUsersOn(cfg config.Config, machine string) map[string]string {
	out := map[string]string{}
	for _, w := range cfg.WorkspacesOn(machine) {
		out[w.Name] = w.LinuxUser()
	}
	return out
}

// workspaceSecretTargetPath is what a report and `secrets list` show: the
// default file when nothing more specific was asked for.
func workspaceSecretTargetPath(t secrets.Target) string {
	return envFilePath(t.EnvFile)
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
	if d.Kind == packages.KindManual {
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
				client, _, err := dialAdmin(cmd.Context(), found.machine)
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

// pushJSON is what a push did, and it never carries a value.
type pushJSON struct {
	Machine   string   `json:"machine"`
	Check     bool     `json:"check"`
	Delivered []string `json:"delivered"`
	// Replaced names the delivered values that were already there with
	// another value: rotated since the last push.
	Replaced []string      `json:"replaced,omitempty"`
	Skipped  []skippedJSON `json:"skipped"`
	// NoValue names the credentials nobody stored a value for.
	NoValue []string `json:"no_value,omitempty"`
	// RemovedFromFile names the workspace secrets removed from their
	// delivery file, by `secrets rm --from-file`.
	RemovedFromFile []string `json:"removed_from_file,omitempty"`
}

type skippedJSON struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

// deliverable is one value this push would write, and where.
type deliverable struct {
	d      credentials.Declared
	secret string
}

func newCredentialsPushCmd(opts *options) *cobra.Command {
	var check, yes bool

	c := &cobra.Command{
		Use:   "push",
		Short: "Deliver the values a machine is missing or holds an old copy of",
		Long: "Only what is missing is written, and a token the CLI delivered " +
			"as an env file that differs from the value stored here: a rotated " +
			"token reaches the machine on the next push. The machine is asked " +
			"only for a SHA-256 of that file, never the value. A file a tool " +
			"keeps for itself is never rewritten once it is there. A login is skipped — nobody " +
			"can push a browser session — and a credential nobody stored a " +
			"value for is named, because a push that quietly does nothing is " +
			"the failure this command exists to prevent.\n\n" +
			"A value is never printed: not in the plan, not in the result, " +
			"not in the command log.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPush(cmd, opts, check, yes)
		},
	}
	c.Flags().BoolVar(&check, "check", false, "a dry run: say what would be written, and write nothing")
	c.Flags().BoolVar(&yes, "yes", false, "deliver without asking")
	return c
}

// wsDeliverable is one workspace secret a push would either write or remove.
type wsDeliverable struct {
	t      secrets.Target
	user   string
	remove bool
}

func runPush(cmd *cobra.Command, opts *options, check, yes bool) error {
	found, err := credentialsOnMachine(cmd.Context(), opts)
	if err != nil {
		return err
	}
	stored, err := storedNames(found.dir)
	if err != nil {
		return err
	}
	wsTargets, err := workspaceTargetsOnMachine(found.dir, found.cfg, found.machine.Name)
	if err != nil {
		return err
	}
	if len(found.wanted) == 0 && len(wsTargets) == 0 {
		return reportPush(cmd, opts, pushJSON{Machine: found.machine.Name, Check: check})
	}

	client, _, err := dialAdmin(cmd.Context(), found.machine)
	if err != nil {
		return err
	}
	defer client.Close()

	present, digests := map[string]bool{}, map[string]string{}
	if len(found.wanted) > 0 {
		present, digests, err = credentials.Look(cmd.Context(), client, found.wanted)
		if err != nil {
			return err
		}
	}

	report := pushJSON{Machine: found.machine.Name, Check: check}
	var work []deliverable
	for _, d := range found.wanted {
		switch {
		case d.Kind == packages.KindManual:
			report.Skipped = append(report.Skipped, skippedJSON{
				credentials.Key(d),
				"a login cannot be pushed: run `" + credentials.LoginCommand(d) + "`",
			})
		case present[credentials.Key(d)]:
			if !rotated(found.dir, d, stored, digests[credentials.Key(d)]) {
				report.Skipped = append(report.Skipped, skippedJSON{credentials.Key(d), "already there"})
				continue
			}
			report.Replaced = append(report.Replaced, credentials.Key(d))
			work = append(work, deliverable{d: d, secret: secretName(d, stored)})
		case credentials.Destination(d) == "":
			report.Skipped = append(report.Skipped, skippedJSON{
				credentials.Key(d),
				fmt.Sprintf("package %q says neither `env` nor `path`, so there is nowhere to deliver it", d.Package),
			})
		case !stored[secretName(d, stored)]:
			report.NoValue = append(report.NoValue, credentials.Key(d))
		default:
			work = append(work, deliverable{d: d, secret: secretName(d, stored)})
		}
	}

	users := linuxUsersOn(found.cfg, found.machine.Name)
	var wsWork []wsDeliverable
	for _, t := range wsTargets {
		switch {
		case t.PendingRemoval:
			wsWork = append(wsWork, wsDeliverable{t: t, user: users[t.Workspace], remove: true})
		case !stored[t.Key()]:
			report.NoValue = append(report.NoValue, t.Key())
		default:
			wsWork = append(wsWork, wsDeliverable{t: t, user: users[t.Workspace]})
		}
	}

	notes := cmd.OutOrStdout()
	if opts.format == formatJSON {
		notes = cmd.ErrOrStderr()
	}
	for _, w := range work {
		fmt.Fprintf(notes, "%s -> %s\n", credentials.Key(w.d), credentials.Destination(w.d))
	}
	for _, w := range wsWork {
		for _, left := range w.t.LeftEnvFiles {
			fmt.Fprintf(notes, "%s: remove from %s\n", w.t.Key(), envFilePath(left))
		}
		if w.remove {
			fmt.Fprintf(notes, "%s: remove from %s\n", w.t.Key(), workspaceSecretTargetPath(w.t))
			continue
		}
		fmt.Fprintf(notes, "%s -> %s\n", w.t.Key(), workspaceSecretTargetPath(w.t))
	}

	total := len(work) + len(wsWork)
	if !check && total > 0 && !yes {
		ok, err := confirm(cmd.InOrStdin(), notes, fmt.Sprintf("Deliver %d value(s) to %s?", total, found.machine.Name))
		if err != nil {
			return err
		}
		if !ok {
			return errDeclined
		}
	}

	for _, w := range work {
		if !check {
			value, err := secrets.Get(found.dir, w.secret)
			if err != nil {
				return err
			}
			if err := credentials.Push(cmd.Context(), client, w.d, value); err != nil {
				return err
			}
		}
		report.Delivered = append(report.Delivered, credentials.Key(w.d))
	}

	for _, w := range wsWork {
		rel := envFilePath(w.t.EnvFile)
		if !check {
			if err := leaveEnvFiles(cmd.Context(), client, w.user, w.t); err != nil {
				return err
			}
		}
		if w.remove {
			if !check {
				if err := credentials.RemoveWorkspaceEnv(cmd.Context(), client, w.user, rel, w.t.Name, w.t.EnvFile != ""); err != nil {
					return err
				}
				if err := secrets.RemoveTarget(found.dir, w.t.Workspace, w.t.Name); err != nil {
					return err
				}
			}
			report.RemovedFromFile = append(report.RemovedFromFile, w.t.Key())
			continue
		}
		if !check {
			value, err := secrets.Get(found.dir, w.t.Key())
			if err != nil {
				return err
			}
			if err := credentials.PushWorkspaceEnv(cmd.Context(), client, w.user, rel, w.t.Name, value, w.t.EnvFile != ""); err != nil {
				return err
			}
			if len(w.t.LeftEnvFiles) > 0 {
				w.t.LeftEnvFiles = nil
				if err := secrets.SetTarget(found.dir, w.t); err != nil {
					return err
				}
			}
		}
		report.Delivered = append(report.Delivered, w.t.Key())
	}
	record(opts, target{machine: found.machine}, pushCommandLine(check), true)

	if err := reportPush(cmd, opts, report); err != nil {
		return err
	}
	if len(report.NoValue) > 0 {
		return fmt.Errorf("%d credential(s) have no value stored: %s",
			len(report.NoValue), strings.Join(report.NoValue, ", "))
	}
	return nil
}

// rotated says whether the value stored here differs from the file on the
// machine. Only a file this CLI owns can be rotated — the sourced env file it
// writes and nothing else touches. A file a tool keeps (a session it renews,
// a config it adds to) differs from the stored value for good reasons, and
// writing the stored one back would undo them. A machine that could not hash
// the file, or a value that cannot be read here, is never taken for a
// difference: an unknown is not a change.
func rotated(dir string, d credentials.Declared, stored map[string]bool, digest string) bool {
	name := secretName(d, stored)
	if digest == "" || !credentials.OwnsFile(d) || !stored[name] {
		return false
	}
	value, err := secrets.Get(dir, name)
	if err != nil {
		return false
	}
	return credentials.Digest(d, value) != digest
}

func pushCommandLine(check bool) string {
	if check {
		return "credentials push --check"
	}
	return "credentials push"
}

func reportPush(cmd *cobra.Command, opts *options, report pushJSON) error {
	if opts.format == formatJSON {
		return writeJSON(cmd.OutOrStdout(), report)
	}

	out := cmd.OutOrStdout()
	verb := "delivered"
	if report.Check {
		verb = "would deliver"
	}
	for _, key := range report.Delivered {
		if slices.Contains(report.Replaced, key) {
			fmt.Fprintf(out, "%s %s, replacing the older value on the machine\n", verb, key)
			continue
		}
		fmt.Fprintf(out, "%s %s\n", verb, key)
	}
	for _, s := range report.Skipped {
		fmt.Fprintf(out, "skipped %s: %s\n", s.Key, s.Reason)
	}
	for _, key := range report.NoValue {
		fmt.Fprintf(out, "no value stored for %s: run `devmachine secrets set %s`\n", key, nameOf(key))
	}
	removeVerb := "removed"
	if report.Check {
		removeVerb = "would remove"
	}
	for _, key := range report.RemovedFromFile {
		fmt.Fprintf(out, "%s %s from its file\n", removeVerb, key)
	}
	if len(report.Delivered) == 0 && len(report.NoValue) == 0 && len(report.RemovedFromFile) == 0 {
		fmt.Fprintln(out, "nothing to deliver")
	}
	return nil
}

// nameOf is the credential's own name inside a key, which is what `secrets
// set` takes.
func nameOf(key string) string {
	if _, name, ok := strings.Cut(key, "/"); ok {
		return name
	}
	return key
}
