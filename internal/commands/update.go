package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/credentials"
	"github.com/mydevmachine/devmachine/internal/doctor"
	"github.com/mydevmachine/devmachine/internal/provision"
	"github.com/mydevmachine/devmachine/internal/release"
	"github.com/mydevmachine/devmachine/internal/selfupdate"
	agentskills "github.com/mydevmachine/devmachine/internal/skills"
	"github.com/spf13/cobra"
)

// The seams `update` reaches the world through. A test replaces each one, so
// no test ever replaces a real binary, runs a real brew or reads a real home.
var (
	executablePath  = os.Executable
	findBrew        = func() (string, error) { return exec.LookPath("brew") }
	selfDownloadURL = selfupdate.DownloadURL
	commandLineArgs = func() []string { return os.Args[1:] }
	execBinary      = func(path string, argv []string) error { return syscall.Exec(path, argv, os.Environ()) }
	localSkills     = func() (agentskills.Installer, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return agentskills.Installer{}, fmt.Errorf("finding your home directory: %w", err)
		}
		return agentskills.Installer{Home: home}, nil
	}
)

// The words a step ends with in the summary.
const (
	stepUpdated    = "updated"
	stepLatest     = "already latest"
	stepOK         = "ok"
	stepNothing    = "nothing to do"
	stepSkipped    = "skipped"
	stepFailed     = "failed"
	updateSteps    = 5
	maxChangeLines = 15
)

type stepResult struct {
	name   string
	status string
	detail string
}

type updateFlags struct {
	skipCLI      bool
	skipPackages bool
	yes          bool
	continueFrom string
}

func newUpdateCmd(opts *options) *cobra.Command {
	var flags updateFlags
	c := &cobra.Command{
		Use:   "update",
		Short: "Bring the CLI, packages pin and skills up to date, then check every machine",
		Long: "In order: updates the CLI itself, pins the newest packages release, refreshes " +
			"the skills on your computer, runs doctor, and runs `sync --check` on every " +
			"reachable machine.\n\n" +
			"Nothing on a machine changes unless you answer yes to the one question at the " +
			"end, or pass --yes. Without a terminal and without --yes, it stops and prints " +
			"the sync command to run later.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.format == formatJSON {
				return errors.New("update is for a person to read; --format json is not supported: " +
					"use `doctor` and `sync --check` with --format json instead")
			}
			u := &updater{cmd: cmd, opts: opts, flags: flags, out: cmd.OutOrStdout()}
			return u.run(cmd.Context())
		},
	}
	c.Flags().BoolVar(&flags.skipCLI, "skip-cli", false, "do not update the CLI itself")
	c.Flags().BoolVar(&flags.skipPackages, "skip-packages", false, "do not move the packages pin")
	c.Flags().BoolVar(&flags.yes, "yes", false, "apply the sync without asking; this changes machines")
	c.Flags().StringVar(&flags.continueFrom, selfupdate.ContinueFlag, "", "the version the CLI was updated from")
	_ = c.Flags().MarkHidden(selfupdate.ContinueFlag)
	return c
}

type updater struct {
	cmd   *cobra.Command
	opts  *options
	flags updateFlags
	out   io.Writer

	dir       string
	cfg       config.Config
	cfgErr    error
	machines  []config.Machine
	reachable map[string]bool
	results   []stepResult
}

func (u *updater) run(ctx context.Context) error {
	dir, _, err := config.Dir(u.opts.configDir)
	if err != nil {
		return err
	}
	u.dir = dir
	if err := u.loadConfig(); err != nil {
		return err
	}

	u.header(1, "CLI")
	cli, handedOver := u.cliStep(ctx)
	if handedOver {
		return nil
	}
	u.finish(cli)

	u.header(2, "Packages")
	u.finish(u.packagesStep(ctx))

	u.header(3, "Skills")
	u.finish(u.skillsStep(ctx))

	u.header(4, "Doctor")
	u.finish(u.doctorStep(ctx))

	u.header(5, "Sync check")
	u.finish(u.syncStep(ctx))

	return u.summary()
}

// loadConfig reads the configuration once, up front. Only an unknown
// --machine stops the whole run: a missing or broken configuration is what
// the steps that need it report.
func (u *updater) loadConfig() error {
	u.cfg, u.cfgErr = loadConfig(u.opts)
	if u.cfgErr != nil {
		return nil
	}
	if u.opts.machine != "" {
		m, err := u.cfg.Machine(u.opts.machine)
		if err != nil {
			return err
		}
		u.machines = []config.Machine{m}
		return nil
	}
	u.machines = u.cfg.Machines
	return nil
}

// configProblem is why a step that needs the configuration cannot run, or
// nil when it can.
func (u *updater) configProblem(name string) *stepResult {
	switch {
	case u.cfgErr == nil:
		return nil
	case errors.Is(u.cfgErr, os.ErrNotExist):
		return &stepResult{name, stepSkipped, "no configuration yet: run `devmachine setup`"}
	default:
		return &stepResult{name, stepFailed, u.cfgErr.Error()}
	}
}

func (u *updater) header(n int, title string) {
	if n > 1 {
		u.say("")
	}
	u.say(fmt.Sprintf("==> %d/%d %s", n, updateSteps, title))
}

func (u *updater) say(line string) {
	fmt.Fprintln(u.out, line)
}

func (u *updater) finish(r stepResult) {
	line := "  " + r.status
	if r.detail != "" {
		line += ": " + r.detail
	}
	u.say(line)
	u.results = append(u.results, r)
}

func (u *updater) summary() error {
	u.say("")
	u.say("Summary")
	var failed []string
	for _, r := range u.results {
		line := fmt.Sprintf("  %-9s %s", r.name, r.status)
		if r.detail != "" {
			line += " (" + r.detail + ")"
		}
		u.say(line)
		if r.status == stepFailed {
			failed = append(failed, r.name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("update: %s failed", strings.Join(failed, ", "))
	}
	return nil
}

// cliStep updates the CLI itself. When it replaced the binary it starts the
// new one for the remaining steps and reports handedOver: the rest of this
// process is the old code, and the new code is what should run them.
func (u *updater) cliStep(ctx context.Context) (stepResult, bool) {
	const name = "cli"
	if u.flags.continueFrom != "" {
		if release.Newer(version, u.flags.continueFrom) {
			return stepResult{name, stepUpdated, u.flags.continueFrom + " → " + version}, false
		}
		return stepResult{name, stepFailed, fmt.Sprintf(
			"still %s after the upgrade; see https://github.com/mydevmachine/devmachine/releases", version)}, false
	}
	if u.flags.skipCLI {
		return stepResult{name, stepSkipped, "--skip-cli"}, false
	}
	if _, ok := release.Compare(version, "0"); !ok {
		return stepResult{name, stepSkipped, fmt.Sprintf("%q is a development build; it is never replaced", version)}, false
	}

	latest, err := releaseCache().Refresh(ctx, cacheKeyCLI, fetchLatestCLI)
	if err != nil {
		return stepResult{name, stepFailed, err.Error()}, false
	}
	if !release.Newer(latest, version) {
		return stepResult{name, stepLatest, version}, false
	}
	u.say(fmt.Sprintf("  %s → %s", version, latest))

	newBinary, err := u.installCLI(ctx, latest)
	if err != nil {
		return stepResult{name, stepFailed, err.Error()}, false
	}

	u.say("  starting the new version for the remaining steps")
	argv := append([]string{newBinary}, selfupdate.ContinueArgs(commandLineArgs(), version)...)
	if err := execBinary(newBinary, argv); err != nil {
		return stepResult{name, stepFailed, fmt.Sprintf(
			"installed %s, but could not start it (%v): run `devmachine update --skip-cli`", latest, err)}, false
	}
	return stepResult{}, true
}

// installCLI puts release tag in place of the running binary and returns the
// path to start the new one from.
//
// Homebrew owns a binary it installed, so that one is upgraded through brew;
// writing over it by hand would leave brew believing in a version that is
// not there.
func (u *updater) installCLI(ctx context.Context, tag string) (string, error) {
	executable, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("finding the running binary: %w", err)
	}

	brewPath, brewErr := findBrew()
	var brewPrefix string
	brew := selfupdate.Brew{Path: brewPath, Stdout: u.out, Stderr: u.cmd.ErrOrStderr()}
	if brewErr == nil {
		brewPrefix, _ = brew.Prefix(ctx)
	}

	if prefix := selfupdate.HomebrewPrefix(executable, brewPrefix); prefix != "" {
		if brewErr != nil {
			return "", fmt.Errorf("%s was installed by Homebrew, but brew is not on PATH: upgrade with `brew upgrade %s`",
				executable, selfupdate.Formula)
		}
		u.say("  upgrading through Homebrew")
		if err := brew.Upgrade(ctx); err != nil {
			return "", err
		}
		return selfupdate.BinaryUnder(prefix), nil
	}

	u.say(fmt.Sprintf("  downloading %s for %s/%s", tag, runtime.GOOS, runtime.GOARCH))
	downloader := selfupdate.Downloader{BaseURL: selfDownloadURL, OS: runtime.GOOS, Arch: runtime.GOARCH}
	if err := downloader.Replace(ctx, tag, executable); err != nil {
		return "", err
	}
	return executable, nil
}

// packagesStep moves the pin to the newest packages release. It only edits
// config.yml: no machine sees the new release until a sync.
func (u *updater) packagesStep(ctx context.Context) stepResult {
	const name = "packages"
	if u.flags.skipPackages {
		return stepResult{name, stepSkipped, "--skip-packages"}
	}
	if problem := u.configProblem(name); problem != nil {
		return *problem
	}
	pinned := u.cfg.Packages
	if pinned == "" {
		return stepResult{name, stepSkipped, "no packages release is pinned; `devmachine packages pin` pins the latest"}
	}

	latest, err := releaseCache().Refresh(ctx, cacheKeyPackages, fetchLatestPackages)
	if err != nil {
		return stepResult{name, stepFailed, err.Error()}
	}
	if !release.Newer(latest, pinned) {
		return stepResult{name, stepLatest, pinned}
	}

	u.say(fmt.Sprintf("  packages %s → %s", pinned, latest))
	if _, err := pinPackages(ctx, u.dir, u.cfg, latest); err != nil {
		return stepResult{name, stepFailed, err.Error()}
	}
	u.cfg.Packages = latest
	return stepResult{name, stepUpdated, pinned + " → " + latest}
}

// skillsStep is `devmachine skills update --yes`, in this process.
//
// It runs after the pin moves, so the official skills come from the release
// the machines are about to get rather than the one they are leaving.
func (u *updater) skillsStep(ctx context.Context) stepResult {
	const name = "skills"
	installer, err := localSkills()
	if err != nil {
		return stepResult{name, stepFailed, err.Error()}
	}
	records, err := installer.List()
	if err != nil {
		return stepResult{name, stepFailed, err.Error()}
	}
	if len(records) == 0 {
		return stepResult{name, stepSkipped, "no skills are managed here; `devmachine skills add` installs them"}
	}

	results, err := refreshSkills(ctx, u.dir, installer, records)
	if err != nil {
		return stepResult{name, stepFailed, err.Error()}
	}
	changed := 0
	for _, r := range results {
		changed += r.Changed
		origin := r.Origin
		if origin == "" {
			origin = r.Source
		}
		u.say(fmt.Sprintf("  %s from %s for %s", strings.Join(r.Skills, ", "), origin, joinAgents(r.Agents)))
	}
	if changed == 0 {
		return stepResult{name, stepLatest, ""}
	}
	return stepResult{name, stepUpdated, fmt.Sprintf("%d filesystem change(s)", changed)}
}

// doctorStep runs doctor on every machine it acts on. A failed check is shown
// and does not stop the run; an unreachable machine only loses its sync
// check, since there is nothing to check it against.
func (u *updater) doctorStep(ctx context.Context) stepResult {
	const name = "doctor"
	u.reachable = map[string]bool{}
	if problem := u.configProblem(name); problem != nil {
		return *problem
	}
	if len(u.machines) == 0 {
		return stepResult{name, stepSkipped, "no machine is configured"}
	}

	var failures []string
	for _, m := range u.machines {
		machineOpts := *u.opts
		machineOpts.machine = m.Name
		var wanted []credentials.Declared
		if found, err := credentialsOnMachine(ctx, &machineOpts); err == nil {
			wanted = found.wanted
		}
		checks := doctor.RunWithScanner(ctx, u.dir, m.Name, dial, scanHostKey, wanted)

		u.say("  machine " + m.Name + ":")
		width := 18
		for _, c := range checks {
			width = max(width, len(c.Name))
		}
		failed := 0
		for _, c := range checks {
			u.say(fmt.Sprintf("    %-4s  %-*s  %s", c.Status, width, c.Name, c.Detail))
			if c.Status == doctor.StatusFail {
				failed++
			}
		}
		u.reachable[m.Name] = reachable(m, checks)
		if failed > 0 {
			failures = append(failures, fmt.Sprintf("%d on %s", failed, m.Name))
		}
	}
	if len(failures) > 0 {
		return stepResult{name, stepFailed, "failed checks: " + strings.Join(failures, ", ")}
	}
	return stepResult{name, stepOK, fmt.Sprintf("%d machine(s)", len(u.machines))}
}

// reachable says whether a sync could reach a machine, from its doctor
// checks: a configuration, host key or connection failure means it could
// not. A self machine has no connection check; its operating system check
// is what dials it.
func reachable(m config.Machine, checks []doctor.Check) bool {
	blocking := map[string]bool{
		doctor.CheckConfiguration: true,
		doctor.CheckHostKey:       true,
		doctor.CheckConnection:    true,
	}
	if m.Self {
		blocking[doctor.CheckOperatingSystem] = true
	}
	for _, c := range checks {
		if blocking[c.Name] && c.Status == doctor.StatusFail {
			return false
		}
	}
	return true
}

// syncStep runs `sync --check` on every reachable machine, says what would
// change, and asks one question before applying any of it.
func (u *updater) syncStep(ctx context.Context) stepResult {
	const name = "sync"
	if problem := u.configProblem(name); problem != nil {
		return *problem
	}
	if len(u.machines) == 0 {
		return stepResult{name, stepSkipped, "no machine is configured"}
	}

	var pending, failed []string
	for _, m := range u.machines {
		if !u.reachable[m.Name] {
			u.say("  " + m.Name + ": unreachable, so its sync check is skipped")
			continue
		}
		changes, err := u.checkMachine(ctx, m.Name)
		if err != nil {
			u.say(fmt.Sprintf("  %s: the sync check failed: %v", m.Name, err))
			failed = append(failed, m.Name)
			continue
		}
		if changes > 0 {
			pending = append(pending, m.Name)
		}
	}

	if len(pending) == 0 {
		if len(failed) > 0 {
			return stepResult{name, stepFailed, "the check failed on " + strings.Join(failed, ", ")}
		}
		return stepResult{name, stepNothing, "every machine matches the configuration"}
	}

	apply, err := u.approve()
	if err != nil {
		return stepResult{name, stepFailed, err.Error()}
	}
	if !apply {
		u.say("")
		u.say("  Nothing was applied. To apply it later:")
		var commands []string
		for _, m := range pending {
			command := u.syncCommand(m)
			commands = append(commands, "`"+command+"`")
			u.say("    " + command)
		}
		return stepResult{name, stepSkipped, "not applied; run " + strings.Join(commands, ", ")}
	}

	var applied []string
	for _, m := range pending {
		if err := u.applyMachine(ctx, m); err != nil {
			u.say(fmt.Sprintf("  %s: the sync failed: %v", m, err))
			failed = append(failed, m)
			continue
		}
		applied = append(applied, m)
	}
	if len(failed) > 0 {
		return stepResult{name, stepFailed, "failed on " + strings.Join(failed, ", ")}
	}
	return stepResult{name, stepUpdated, "applied to " + strings.Join(applied, ", ")}
}

// checkMachine runs a dry run on one machine and prints what it would change.
// Ansible's own output is kept back unless the run fails: the changed tasks
// are what somebody deciding yes or no needs to read.
func (u *updater) checkMachine(ctx context.Context, machine string) (int, error) {
	machineOpts := *u.opts
	machineOpts.machine = machine
	prep, err := prepareSync(ctx, &machineOpts)
	if err != nil {
		return 0, err
	}

	output := &bytes.Buffer{}
	result, err := prep.apply(ctx, &machineOpts, true, nil, output)
	if err != nil {
		u.say(indent(tail(output.String(), 20), "    "))
		return 0, err
	}
	if result.Changed == 0 {
		u.say("  " + machine + ": nothing would change")
		return 0, nil
	}

	u.say(fmt.Sprintf("  %s: %d change(s) would be made", machine, result.Changed))
	tasks := provision.ChangedTasks(output.String())
	for i, task := range tasks {
		if i == maxChangeLines {
			u.say(fmt.Sprintf("    … and %d more", len(tasks)-maxChangeLines))
			break
		}
		u.say("    ~ " + task)
	}
	return result.Changed, nil
}

// approve is the one question `update` asks. Nobody at a terminal, and no
// --yes, is a no: a machine never changes on an answer nobody gave.
func (u *updater) approve() (bool, error) {
	if u.flags.yes {
		return true, nil
	}
	in := u.cmd.InOrStdin()
	if !fromATerminal(in) {
		return false, nil
	}
	return confirm(in, u.out, "Apply these changes with sync?")
}

func (u *updater) applyMachine(ctx context.Context, machine string) error {
	machineOpts := *u.opts
	machineOpts.machine = machine
	prep, err := prepareSync(ctx, &machineOpts)
	if err != nil {
		return err
	}
	u.say("")
	u.say("==> sync " + machine)
	result, err := prep.apply(ctx, &machineOpts, false, nil, u.out)
	if err != nil {
		return err
	}
	u.say(fmt.Sprintf("  %s: ok=%d changed=%d failed=%d", machine, result.Ok, result.Changed, result.Failed))
	return nil
}

// syncCommand is the exact command that applies what update left pending on
// one machine. With several machines, `sync` alone refuses to guess.
func (u *updater) syncCommand(machine string) string {
	command := "devmachine"
	if u.opts.configDir != "" {
		command += " --config " + u.opts.configDir
	}
	command += " sync"
	if len(u.cfg.Machines) > 1 {
		command += " --machine " + machine
	}
	return command
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
