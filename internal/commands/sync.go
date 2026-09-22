package commands

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/provision"
	"github.com/adevmachine/cli/internal/remote"
	"github.com/adevmachine/cli/internal/repo"
	"github.com/spf13/cobra"
)

// ansibleProvisioner is the real thing a sync applies with, and provisionerFor
// is the seam a test replaces so a sync runs without a machine.
func ansibleProvisioner(client remote.Client) provision.Provisioner {
	return &provision.Ansible{Client: client}
}

var provisionerFor = ansibleProvisioner

func newSyncCmd(opts *options) *cobra.Command {
	var (
		check bool
		yes   bool
		tags  []string
	)

	c := &cobra.Command{
		Use:   "sync",
		Short: "Put a machine into the state the configuration describes",
		Long: "It reads the packages each machine and workspace asks for, " +
			"sends them, and runs Ansible on the machine, streaming the " +
			"output as it arrives.\n\n" +
			"The plan is printed before anything happens. With --check the " +
			"machine reports what would change and changes nothing, and " +
			"nothing is written to the lock.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd, opts, check, yes, tags)
		},
	}
	c.Flags().BoolVar(&check, "check", false, "a dry run: the machine reports what would change and changes nothing")
	c.Flags().BoolVar(&yes, "yes", false, "apply without asking")
	c.Flags().StringSliceVar(&tags, "tags", nil, "only the packages named, by name")
	return c
}

func runSync(cmd *cobra.Command, opts *options, check, yes bool, tags []string) error {
	dir, _, err := config.Dir(opts.configDir)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}
	machine, err := cfg.Machine(opts.machine)
	if err != nil {
		return err
	}

	store, err := openStore(cmd.Context(), dir, cfg.Packages)
	if err != nil {
		return err
	}
	plan, err := packages.ResolveMachine(store, cfg, machine, version)
	if err != nil {
		return err
	}
	if err := validateLocalPackages(plan); err != nil {
		return err
	}

	// In JSON the document on stdout is the contract, so the plan, the prompt
	// and the machine's own output are diagnostics and go to stderr.
	notes := cmd.OutOrStdout()
	if opts.format == formatJSON {
		notes = cmd.ErrOrStderr()
	}

	summary := provision.Summary(plan)
	for _, line := range summary {
		if _, err := fmt.Fprintln(notes, line); err != nil {
			return err
		}
	}

	if !yes && !check {
		ok, err := confirm(cmd.InOrStdin(), notes, "Apply this to "+machine.Name+"?")
		if err != nil {
			return err
		}
		if !ok {
			return errDeclined
		}
	}

	client, _, err := dial(cmd.Context(), machine, "")
	if err != nil {
		return err
	}
	defer client.Close()

	result, runErr := provisionerFor(client).Apply(cmd.Context(), plan, provision.Options{
		Check: check, Tags: tags, Out: notes,
	})
	record(opts, target{machine: machine}, syncCommandLine(check, tags), runErr == nil)
	if runErr != nil {
		return runErr
	}

	// A dry run changed nothing, so recording it as applied would make the
	// lock claim something nobody did.
	if !check {
		lock, err := packages.LoadLock(dir)
		if err != nil {
			return err
		}
		if err := packages.SaveLock(dir, lock.WithPlan(plan, store, time.Now())); err != nil {
			return err
		}
		repo.AutoCommit(cmd.Context(), dir, "chore(config): lock packages for "+machine.Name)
	}
	return reportSync(cmd, opts, machine.Name, check, summary, result)
}

// validateLocalPackages checks the operator's own recipes before anything is
// sent.
//
// Only the local ones: a published package was checked when it was released,
// and a local one has never been checked by anybody.
func validateLocalPackages(plan packages.MachinePlan) error {
	var (
		lines []string
		seen  = map[string]bool{}
	)

	for _, resolved := range append([]packages.Resolved{plan.OnMachine}, plan.Workspaces...) {
		for _, found := range resolved.Ordered {
			if found.Source != packages.SourceLocal || seen[found.Manifest.Path] {
				continue
			}
			seen[found.Manifest.Path] = true

			problems, err := packages.Validate(found.Manifest.Path)
			if err != nil {
				return err
			}
			for _, p := range problems {
				lines = append(lines, fmt.Sprintf("%s: %s", found.Manifest.Path, p.Error()))
			}
		}
	}

	if len(lines) > 0 {
		return fmt.Errorf("%d problem(s) in your own packages:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	return nil
}

// syncCommandLine is what the command log records, so a line in it can be read
// back as the command somebody ran.
func syncCommandLine(check bool, tags []string) string {
	command := "sync"
	if check {
		command += " --check"
	}
	if len(tags) > 0 {
		command += " --tags " + strings.Join(tags, ",")
	}
	return command
}

func reportSync(cmd *cobra.Command, opts *options, machine string, check bool, plan []string, result provision.Result) error {
	if opts.format == formatJSON {
		return writeJSON(cmd.OutOrStdout(), struct {
			Machine string           `json:"machine"`
			Check   bool             `json:"check"`
			Plan    []string         `json:"plan"`
			Result  provision.Result `json:"result"`
			Locked  bool             `json:"locked"`
		}{machine, check, plan, result, !check})
	}

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "\n%s: ok=%d changed=%d failed=%d\n",
		machine, result.Ok, result.Changed, result.Failed); err != nil {
		return err
	}
	if check {
		return writeLine(out, "a dry run: nothing changed, and the lock was not written")
	}
	return nil
}

func writeLine(out io.Writer, line string) error {
	_, err := fmt.Fprintln(out, line)
	return err
}
