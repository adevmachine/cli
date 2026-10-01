package commands

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/hostkeys"
	"github.com/mydevmachine/devmachine/internal/packages"
	"github.com/mydevmachine/devmachine/internal/provision"
	"github.com/mydevmachine/devmachine/internal/remote"
	"github.com/mydevmachine/devmachine/internal/selfupdate"
	agentskills "github.com/mydevmachine/devmachine/internal/skills"
	"golang.org/x/crypto/ssh"
)

// machineRuns stands in for Ansible on every machine, and remembers each run
// so a test can tell a dry run from a real one.
type machineRuns struct {
	changed int
	output  string
	err     error
	runs    []machineRun
}

type machineRun struct {
	machine string
	check   bool
}

func (m *machineRuns) Apply(_ context.Context, plan packages.MachinePlan, opts provision.Options) (provision.Result, error) {
	m.runs = append(m.runs, machineRun{plan.Machine.Name, opts.Check})
	if opts.Out != nil {
		_, _ = io.WriteString(opts.Out, m.output)
	}
	return provision.Result{Ok: 4, Changed: m.changed}, m.err
}

func (m *machineRuns) applied() []string {
	var out []string
	for _, r := range m.runs {
		if !r.check {
			out = append(out, r.machine)
		}
	}
	return out
}

func (m *machineRuns) checked() []string {
	var out []string
	for _, r := range m.runs {
		if r.check {
			out = append(out, r.machine)
		}
	}
	return out
}

// updateWorld is everything `update` reaches, faked: GitHub, the machine,
// Ansible, the terminal and the skills on your computer.
type updateWorld struct {
	dir  string
	runs *machineRuns
	cli  *int
}

func newUpdateWorld(t *testing.T, pinned, latest string, changed int) updateWorld {
	t.Helper()
	runningVersion(t, "0.7.18")
	cli := stubLatestCLI(t, "v0.7.18", nil)
	t.Cleanup(stubLatestPackagesRelease(t, latest))
	dir := pinnedConfig(t, pinned)
	writeCommandFile(t, filepath.Join(packages.CacheDir(dir, latest), ".checksum"), "fixture\n")
	dialing(t, &recordingRemote{out: "ID=ubuntu\n"})
	runs := &machineRuns{changed: changed}
	t.Cleanup(swap(&provisionerFor, func(remote.Client) provision.Provisioner { return runs }))
	withSkillsHome(t, t.TempDir())
	terminal(t, true)
	return updateWorld{dir: dir, runs: runs, cli: cli}
}

func withSkillsHome(t *testing.T, home string) agentskills.Installer {
	t.Helper()
	installer := agentskills.Installer{Home: home, StateDir: filepath.Join(home, "state")}
	t.Cleanup(swap(&localSkills, func() (agentskills.Installer, error) { return installer, nil }))
	return installer
}

func terminal(t *testing.T, is bool) {
	t.Helper()
	t.Cleanup(swap(&fromATerminal, func(io.Reader) bool { return is }))
}

func pinnedIn(t *testing.T, dir string) string {
	t.Helper()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Packages
}

func summaryLine(t *testing.T, out, step string) string {
	t.Helper()
	_, summary, found := strings.Cut(out, "\nSummary\n")
	if !found {
		t.Fatalf("no summary:\n%s", out)
	}
	for _, line := range strings.Split(summary, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == step {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("the summary has no %q line:\n%s", step, summary)
	return ""
}

func assertSummary(t *testing.T, out, step, status string) {
	t.Helper()
	if line := summaryLine(t, out, step); !strings.Contains(line, status) {
		t.Fatalf("%s: got %q; want %q\n%s", step, line, status, out)
	}
}

func TestUpdateRefusesJSON(t *testing.T) {
	_, err := execute(t, "--format", "json", "update")
	if err == nil || !strings.Contains(err.Error(), "json") {
		t.Fatalf("got %v", err)
	}
}

func TestUpdateRunsEveryStepInOrderAndSummarises(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)

	out, err := execute(t, "--config", w.dir, "update")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	last := -1
	for _, header := range []string{"==> 1/5 CLI", "==> 2/5 Packages", "==> 3/5 Skills", "==> 4/5 Doctor", "==> 5/5 Sync check"} {
		at := strings.Index(out, header)
		if at <= last {
			t.Fatalf("%q is missing or out of order:\n%s", header, out)
		}
		last = at
	}
	assertSummary(t, out, "cli", "already latest")
	assertSummary(t, out, "packages", "updated")
	assertSummary(t, out, "packages", "v16 → v17")
	assertSummary(t, out, "skills", "skipped")
	assertSummary(t, out, "doctor", "ok")
	assertSummary(t, out, "sync", "nothing to do")
	if !strings.Contains(out, "packages v16 → v17") {
		t.Fatalf("the bump is not shown as it happens:\n%s", out)
	}
	if got := pinnedIn(t, w.dir); got != "v17" {
		t.Fatalf("pinned %q", got)
	}
	if !slices.Equal(w.runs.checked(), []string{"main"}) || len(w.runs.applied()) != 0 {
		t.Fatalf("runs: %+v", w.runs.runs)
	}
}

func TestUpdateLeavesTheLatestPinAlone(t *testing.T) {
	for _, pinned := range []string{"v17", "v18"} {
		t.Run(pinned, func(t *testing.T) {
			w := newUpdateWorld(t, pinned, "v17", 0)
			out, err := execute(t, "--config", w.dir, "update")
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			assertSummary(t, out, "packages", "already latest")
			if got := pinnedIn(t, w.dir); got != pinned {
				t.Fatalf("the pin moved from %s to %s", pinned, got)
			}
		})
	}
}

func TestUpdateCommitsThePinInAVersionedConfiguration(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)
	gitRepoDir(t, w.dir)

	if out, err := execute(t, "--config", w.dir, "update"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := lastCommitMessage(t, w.dir); got != "chore(config): pin packages v17" {
		t.Fatalf("last commit %q", got)
	}
}

func TestUpdateSkipPackagesLeavesThePin(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)

	out, err := execute(t, "--config", w.dir, "update", "--skip-packages")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	assertSummary(t, out, "packages", "skipped")
	if got := pinnedIn(t, w.dir); got != "v16" {
		t.Fatalf("pinned %q", got)
	}
}

func TestUpdateRefreshesManagedSkillsFromTheNewPin(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)
	home := t.TempDir()
	installer := withSkillsHome(t, home)
	writeSkillPackage(t, filepath.Join(packages.CacheDir(w.dir, "v16"), "packages"), "devmachine-skills", "use-devmachine", "Old copy.")
	writeSkillPackage(t, filepath.Join(packages.CacheDir(w.dir, "v17"), "packages"), "devmachine-skills", "use-devmachine", "New copy.")
	source, err := skillSource(context.Background(), w.dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(source, []agentskills.Agent{agentskills.AgentClaude}); err != nil {
		t.Fatal(err)
	}

	out, err := execute(t, "--config", w.dir, "update")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	assertSummary(t, out, "skills", "updated")
	body, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "use-devmachine", "SKILL.md"))
	if err != nil || !strings.Contains(string(body), "New copy.") {
		t.Fatalf("the skills did not come from the new pin: %v\n%s", err, body)
	}
}

func TestUpdateShowsWhatWouldChangeAndAppliesOnYes(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 2)
	w.runs.output = "TASK [workspace : Create the account] ***\nchanged: [main]\n"

	out, err := executeWithInput(t, "y\n", "--config", w.dir, "update")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"main: 2 change(s) would be made", "workspace : Create the account", "Apply these changes with sync? [y/N]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if !slices.Equal(w.runs.applied(), []string{"main"}) {
		t.Fatalf("runs: %+v", w.runs.runs)
	}
	assertSummary(t, out, "sync", "updated")
}

func TestUpdateAppliesNothingOnNoOrAnEmptyAnswer(t *testing.T) {
	for name, answer := range map[string]string{"no": "n\n", "empty": "\n", "closed": ""} {
		t.Run(name, func(t *testing.T) {
			w := newUpdateWorld(t, "v17", "v17", 2)

			out, err := executeWithInput(t, answer, "--config", w.dir, "update")
			if err != nil {
				t.Fatalf("declining is not a failure: %v\n%s", err, out)
			}
			if len(w.runs.applied()) != 0 {
				t.Fatalf("applied after %q: %+v", answer, w.runs.runs)
			}
			if !strings.Contains(out, "To apply it later:\n    devmachine --config "+w.dir+" sync\n") {
				t.Fatalf("does not say how to apply later:\n%s", out)
			}
			assertSummary(t, out, "sync", "skipped")
		})
	}
}

func TestUpdateWithoutATerminalNeverAsksAndNeverApplies(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 2)
	terminal(t, false)

	out, err := executeWithInput(t, "y\n", "--config", w.dir, "update")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "[y/N]") {
		t.Fatalf("asked with nobody there:\n%s", out)
	}
	if len(w.runs.applied()) != 0 {
		t.Fatalf("applied without a yes: %+v", w.runs.runs)
	}
	if !strings.Contains(out, "    devmachine --config "+w.dir+" sync\n") {
		t.Fatalf("does not print the command to run:\n%s", out)
	}
}

func TestUpdateYesAppliesWithoutAsking(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 2)
	terminal(t, false)

	out, err := execute(t, "--config", w.dir, "update", "--yes")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "[y/N]") {
		t.Fatalf("--yes still asked:\n%s", out)
	}
	if !slices.Equal(w.runs.applied(), []string{"main"}) {
		t.Fatalf("runs: %+v", w.runs.runs)
	}
}

func twoMachines(t *testing.T) string {
	t.Helper()
	dir := pinnedConfig(t, "v17")
	extra := "  - name: far\n    hosts: [203.0.113.20]\n    key: " + withKey(t, dir, "far") + "\n"
	body, err := os.ReadFile(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(body), "packages: v17\n", extra+"packages: v17\n", 1)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := hostkeys.Open(filepath.Join(dir, config.KnownHostsFileName))
	if err != nil {
		t.Fatal(err)
	}
	key := commandHostKey(t)
	for _, name := range []string{"main", "far"} {
		if err := store.Put(name, 22, key); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(swap(&scanHostKey, func(context.Context, config.Machine) (ssh.PublicKey, string, error) {
		return key, "203.0.113.10", nil
	}))
	return dir
}

func TestUpdateSkipsTheSyncOfAnUnreachableMachine(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 1)
	dir := twoMachines(t)
	client := &recordingRemote{out: "ID=ubuntu\n"}
	t.Cleanup(swap(&dial, func(_ context.Context, m config.Machine, _ string) (remote.Client, string, error) {
		if m.Name == "far" {
			return nil, "", errors.New("no address answered")
		}
		return client, "203.0.113.10", nil
	}))
	terminal(t, false)

	out, err := execute(t, "--config", dir, "update")
	if err != nil {
		t.Fatalf("an unreachable machine alone must not fail update: %v\n%s", err, out)
	}
	if !slices.Equal(w.runs.checked(), []string{"main"}) {
		t.Fatalf("checked %q; far is unreachable", w.runs.checked())
	}
	if !strings.Contains(out, "far: unreachable, so its sync check is skipped") {
		t.Fatalf("does not say why far was skipped:\n%s", out)
	}
	if !strings.Contains(out, " sync --machine main") {
		t.Fatalf("with two machines the command must name one:\n%s", out)
	}
	assertSummary(t, out, "doctor", "machine far unreachable")
	assertSummary(t, out, "sync", "far skipped (unreachable)")
}

func TestUpdateNeverCallsTheOnlyMachineUpToDateWhenItIsUnreachable(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 0)
	t.Cleanup(swap(&dial, func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return nil, "", errors.New("connection refused")
	}))

	out, err := execute(t, "--config", w.dir, "update")
	if err != nil {
		t.Fatalf("an unreachable machine alone must not fail update: %v\n%s", err, out)
	}
	if len(w.runs.runs) != 0 {
		t.Fatalf("ran Ansible against an unreachable machine: %+v", w.runs.runs)
	}
	if !strings.Contains(out, "fail  connection") {
		t.Fatalf("the failed connection is not shown:\n%s", out)
	}
	assertSummary(t, out, "doctor", "machine main unreachable")
	assertSummary(t, out, "sync", "skipped")
	assertSummary(t, out, "sync", "main skipped (unreachable)")
	if strings.Contains(summaryLine(t, out, "sync"), "every machine matches") {
		t.Fatalf("an unchecked machine was called up to date:\n%s", out)
	}
}

func TestUpdateCountsDoctorWarningsWithoutFailing(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 0)
	body, err := os.ReadFile(filepath.Join(w.dir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	withLogin := strings.Replace(string(body), "packages: v17\n", "    packages: [gh-login]\npackages: v17\n", 1)
	if err := os.WriteFile(filepath.Join(w.dir, "config.yml"), []byte(withLogin), 0o600); err != nil {
		t.Fatal(err)
	}
	writeCredentialPackage(t, w.dir, "gh-login", packages.ScopeMachine,
		"  - name: gh\n    kind: manual\n    scope: machine\n"+
			"    command: gh auth login\n    stored_at: ~/.config/gh/hosts.yml\n")
	writeCommandFile(t, filepath.Join(packages.LocalDir(w.dir), "gh-login", "tasks", "main.yml"), "---\n[]\n")

	out, err := execute(t, "--config", w.dir, "update")
	if err != nil {
		t.Fatalf("a doctor warning must not fail update: %v\n%s", err, out)
	}
	if !strings.Contains(out, "warn  credential: gh") || !strings.Contains(out, "devmachine login gh") {
		t.Fatalf("the warning and its fix are not shown:\n%s", out)
	}
	assertSummary(t, out, "doctor", "1 warning(s)")
	if !slices.Equal(w.runs.checked(), []string{"main"}) {
		t.Fatalf("a warning kept the sync check from running: %q", w.runs.checked())
	}
}

func TestUpdateMachineFlagActsOnOneMachine(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 0)
	dir := twoMachines(t)

	out, err := execute(t, "--config", dir, "--machine", "far", "update")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !slices.Equal(w.runs.checked(), []string{"far"}) {
		t.Fatalf("checked %q", w.runs.checked())
	}
	if strings.Contains(out, "machine main") {
		t.Fatalf("doctor looked at main:\n%s", out)
	}
}

func TestUpdateRefusesAnUnknownMachine(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 0)
	if _, err := execute(t, "--config", w.dir, "--machine", "nope", "update"); err == nil {
		t.Fatal("an unknown machine was accepted")
	}
}

func TestUpdateSkipCLINeverAsksGitHubAboutTheCLI(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 0)

	out, err := execute(t, "--config", w.dir, "update", "--skip-cli")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	assertSummary(t, out, "cli", "skipped")
	if *w.cli != 0 {
		t.Fatal("--skip-cli still asked for the latest CLI")
	}
}

func TestUpdateNeverReplacesADevelopmentBuild(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 0)
	runningVersion(t, "dev")

	out, err := execute(t, "--config", w.dir, "update")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	assertSummary(t, out, "cli", "skipped")
}

func TestUpdateGoesOnWhenTheCLILookupFails(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)
	stubLatestCLI(t, "", errors.New("no route to host"))

	out, err := execute(t, "--config", w.dir, "update")
	if err == nil {
		t.Fatalf("a failed step must fail the command:\n%s", out)
	}
	assertSummary(t, out, "cli", "failed")
	assertSummary(t, out, "packages", "updated")
	assertSummary(t, out, "sync", "nothing to do")
}

// selfUpdateWorld is a CLI one release behind, installed at a path in a
// temporary directory, with GitHub answering from a test server.
type selfUpdateWorld struct {
	updateWorld
	binary string
	execed *[]string
}

func newSelfUpdateWorld(t *testing.T, archiveBody string, tamper bool) selfUpdateWorld {
	t.Helper()
	return newSelfUpdateWorldPinned(t, "v17", "v17", archiveBody, tamper)
}

func newSelfUpdateWorldPinned(t *testing.T, pinned, latest, archiveBody string, tamper bool) selfUpdateWorld {
	t.Helper()
	w := newUpdateWorld(t, pinned, latest, 0)
	runningVersion(t, "0.7.17")

	archive := testTarball(t, archiveBody)
	sum := sha256.Sum256(archive)
	if tamper {
		archive = testTarball(t, "tampered")
	}
	name := selfupdate.ArchiveName("v0.7.18", runtime.GOOS, runtime.GOARCH)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0.7.18/checksums.txt":
			fmt.Fprintf(rw, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		case "/v0.7.18/" + name:
			_, _ = rw.Write(archive)
		default:
			http.NotFound(rw, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(swap(&selfDownloadURL, server.URL))

	binary := filepath.Join(t.TempDir(), "devmachine")
	writeCommandFile(t, binary, "old binary")
	t.Cleanup(swap(&executablePath, func() (string, error) { return binary, nil }))
	t.Cleanup(swap(&findBrew, func() (string, error) { return "", errors.New("no brew") }))
	t.Cleanup(swap(&commandLineArgs, func() []string { return []string{"--config", w.dir, "update", "--yes"} }))

	var execed []string
	t.Cleanup(swap(&execBinary, func(path string, argv []string) error {
		execed = append([]string{path}, argv...)
		return nil
	}))
	return selfUpdateWorld{updateWorld: w, binary: binary, execed: &execed}
}

func testTarball(t *testing.T, body string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "devmachine", Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUpdateReplacesTheBinaryAndHandsOverToIt(t *testing.T) {
	w := newSelfUpdateWorld(t, "new binary", false)

	out, err := execute(t, "--config", w.dir, "update", "--yes")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if body, _ := os.ReadFile(w.binary); string(body) != "new binary" {
		t.Fatalf("the binary was not replaced: %q", body)
	}
	want := []string{w.binary, w.binary, "--config", w.dir, "update", "--yes", "--continue-after-self-update=0.7.17"}
	if !slices.Equal(*w.execed, want) {
		t.Fatalf("started %q; want %q", *w.execed, want)
	}
	if strings.Contains(out, "==> 2/5") || len(w.runs.runs) != 0 {
		t.Fatalf("the old binary went on after handing over:\n%s", out)
	}
}

func TestUpdateRefusesADownloadWithTheWrongChecksum(t *testing.T) {
	w := newSelfUpdateWorld(t, "new binary", true)

	out, err := execute(t, "--config", w.dir, "update")
	if err == nil {
		t.Fatalf("a checksum mismatch must fail:\n%s", out)
	}
	if body, _ := os.ReadFile(w.binary); string(body) != "old binary" {
		t.Fatalf("the binary changed: %q", body)
	}
	if len(*w.execed) != 0 {
		t.Fatal("started a binary after a mismatch")
	}
	assertSummary(t, out, "cli", "failed")
	if !strings.Contains(out, "checksum mismatch") {
		t.Fatalf("does not say why:\n%s", out)
	}
	assertSummary(t, out, "sync", "nothing to do")
}

func TestUpdateUpgradesThroughHomebrewWhenHomebrewInstalledIt(t *testing.T) {
	w := newSelfUpdateWorld(t, "new binary", false)
	root := t.TempDir()
	prefix := filepath.Join(root, "brew")
	binary := filepath.Join(prefix, "Cellar", "devmachine", "0.7.17", "bin", "devmachine")
	writeCommandFile(t, binary, "old binary")
	log := filepath.Join(root, "brew.log")
	fake := filepath.Join(root, "bin", "brew")
	writeCommandFile(t, fake, fmt.Sprintf("#!/bin/sh\necho \"$*\" >> %q\n[ \"$1\" = --prefix ] && echo %q\nexit 0\n", log, prefix))
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(swap(&executablePath, func() (string, error) { return binary, nil }))
	t.Cleanup(swap(&findBrew, func() (string, error) { return fake, nil }))

	out, err := execute(t, "--config", w.dir, "update")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "upgrade "+selfupdate.Formula) {
		t.Fatalf("brew was not asked to upgrade: %s", calls)
	}
	if body, _ := os.ReadFile(binary); string(body) != "old binary" {
		t.Fatal("a Homebrew binary was overwritten by hand")
	}
	if got := (*w.execed)[0]; got != filepath.Join(prefix, "bin", "devmachine") {
		t.Fatalf("started %q; want Homebrew's link", got)
	}
}

func TestUpdateAfterTheHandOverSaysWhatChanged(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 0)

	out, err := execute(t, "--config", w.dir, "update", "--continue-after-self-update=0.7.17")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	assertSummary(t, out, "cli", "updated")
	assertSummary(t, out, "cli", "0.7.17 → 0.7.18")
	if *w.cli != 0 {
		t.Fatal("the new binary asked GitHub again")
	}
}

func TestUpdateAfterAHandOverToTheSameVersionFails(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 0)

	out, err := execute(t, "--config", w.dir, "update", "--continue-after-self-update=0.7.18")
	if err == nil {
		t.Fatalf("an upgrade that changed nothing must fail:\n%s", out)
	}
	assertSummary(t, out, "cli", "failed")
}

func TestUpdateShowsTheEndOfAFailedSyncCheckAndAsksNothing(t *testing.T) {
	w := newUpdateWorld(t, "v17", "v17", 1)
	w.runs.output = "TASK [zsh : Install zsh] ***\nfatal: [main]: FAILED! => no package zsh\n"
	w.runs.err = errors.New("ansible-playbook exited 2")

	out, err := executeWithInput(t, "y\n", "--config", w.dir, "update")
	if err == nil {
		t.Fatalf("a failed check must fail the command:\n%s", out)
	}
	if !strings.Contains(out, "    fatal: [main]: FAILED! => no package zsh") {
		t.Fatalf("does not show why the check failed:\n%s", out)
	}
	if strings.Contains(out, "[y/N]") || len(w.runs.applied()) != 0 {
		t.Fatalf("offered or applied a sync after a failed check:\n%s", out)
	}
	assertSummary(t, out, "sync", "failed")
}

func assertOnlyTheCLIWasTouched(t *testing.T, w updateWorld, out string) {
	t.Helper()
	for _, other := range []string{"==> 2/5", "Packages", "Skills", "Doctor", "Sync check", "Summary"} {
		if strings.Contains(out, other) {
			t.Fatalf("--cli-only went past the CLI (%q):\n%s", other, out)
		}
	}
	if len(w.runs.runs) != 0 {
		t.Fatalf("--cli-only reached a machine: %+v", w.runs.runs)
	}
	if got := pinnedIn(t, w.dir); got != "v16" {
		t.Fatalf("--cli-only moved the packages pin to %q", got)
	}
}

func TestUpdateCLIOnlyReplacesTheBinaryAndHandsOverWithCLIOnly(t *testing.T) {
	w := newSelfUpdateWorldPinned(t, "v16", "v17", "new binary", false)
	t.Cleanup(swap(&commandLineArgs, func() []string { return []string{"--config", w.dir, "update", "--cli-only"} }))

	out, err := execute(t, "--config", w.dir, "update", "--cli-only")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if body, _ := os.ReadFile(w.binary); string(body) != "new binary" {
		t.Fatalf("the binary was not replaced: %q", body)
	}
	want := []string{w.binary, w.binary, "--config", w.dir, "update", "--cli-only", "--continue-after-self-update=0.7.17"}
	if !slices.Equal(*w.execed, want) {
		t.Fatalf("started %q; want %q", *w.execed, want)
	}
	if !strings.Contains(out, "0.7.17 → v0.7.18") {
		t.Fatalf("does not say what it updates:\n%s", out)
	}
	assertOnlyTheCLIWasTouched(t, w.updateWorld, out)
}

func TestUpdateCLIOnlyAfterTheHandOverSaysWhatChangedAndStops(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)

	out, err := execute(t, "--config", w.dir, "update", "--cli-only", "--continue-after-self-update=0.7.17")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "updated: 0.7.17 → 0.7.18") {
		t.Fatalf("does not say what changed:\n%s", out)
	}
	if *w.cli != 0 {
		t.Fatal("the new binary asked GitHub again")
	}
	assertOnlyTheCLIWasTouched(t, w, out)
}

func TestUpdateCLIOnlyAfterAHandOverToTheSameVersionFails(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)

	out, err := execute(t, "--config", w.dir, "update", "--cli-only", "--continue-after-self-update=0.7.18")
	if err == nil || !strings.Contains(err.Error(), "still 0.7.18") {
		t.Fatalf("got %v\n%s", err, out)
	}
	assertOnlyTheCLIWasTouched(t, w, out)
}

func TestUpdateCLIOnlyWhenAlreadyLatestDoesNothingElse(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)

	out, err := execute(t, "--config", w.dir, "update", "--cli-only")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "already latest: 0.7.18") {
		t.Fatalf("does not say it is current:\n%s", out)
	}
	assertOnlyTheCLIWasTouched(t, w, out)
}

func TestUpdateCLIOnlyNeedsNoConfiguration(t *testing.T) {
	runningVersion(t, "0.7.18")
	stubLatestCLI(t, "v0.7.18", nil)

	out, err := execute(t, "--config", filepath.Join(t.TempDir(), "missing"), "update", "--cli-only")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestUpdateCLIOnlyFailsOnADevelopmentBuildAndSaysWhatToRun(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)
	runningVersion(t, "dev")

	out, err := execute(t, "--config", w.dir, "update", "--cli-only")
	if err == nil {
		t.Fatalf("a build from source must fail:\n%s", out)
	}
	for _, want := range []string{"built from source", "install.sh"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("%q does not say %q", err, want)
		}
	}
	if *w.cli != 0 {
		t.Fatal("asked GitHub about a build it can never replace")
	}
	assertOnlyTheCLIWasTouched(t, w, out)
}

func TestUpdateCLIOnlyFailsWhenTheLookupFails(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)
	stubLatestCLI(t, "", errors.New("no route to host"))

	out, err := execute(t, "--config", w.dir, "update", "--cli-only")
	if err == nil || !strings.Contains(err.Error(), "no route to host") {
		t.Fatalf("got %v\n%s", err, out)
	}
	assertOnlyTheCLIWasTouched(t, w, out)
}

func TestUpdateCLIOnlyRefusesTheFlagsOfTheOtherSteps(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)
	for _, flag := range []string{"--skip-cli", "--skip-packages", "--yes"} {
		_, err := execute(t, "--config", w.dir, "update", "--cli-only", flag)
		if err == nil || !strings.Contains(err.Error(), flag) {
			t.Fatalf("%s: got %v", flag, err)
		}
	}
	_, err := execute(t, "--config", w.dir, "--machine", "main", "update", "--cli-only")
	if err == nil || !strings.Contains(err.Error(), "--machine") {
		t.Fatalf("--machine: got %v", err)
	}
	if *w.cli != 0 || len(w.runs.runs) != 0 {
		t.Fatal("a refused combination still did something")
	}
}

type cliOnlyJSON struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Method string `json:"method"`
	Status string `json:"status"`
	OK     bool   `json:"ok"`
	Error  string `json:"error"`
}

func decodeCLIOnly(t *testing.T, stdout string) cliOnlyJSON {
	t.Helper()
	var got cliOnlyJSON
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	return got
}

func TestUpdateCLIOnlyJSONReportsTheResultAndLogsToStderr(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)
	binary := filepath.Join(t.TempDir(), "devmachine")
	writeCommandFile(t, binary, "binary")
	t.Cleanup(swap(&executablePath, func() (string, error) { return binary, nil }))
	t.Cleanup(swap(&findBrew, func() (string, error) { return "", errors.New("no brew") }))

	stdout, stderr, err := executeSplit(t, "--config", w.dir, "--format", "json",
		"update", "--cli-only", "--continue-after-self-update=0.7.17")
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	got := decodeCLIOnly(t, stdout)
	want := cliOnlyJSON{From: "0.7.17", To: "0.7.18", Method: "download", Status: "updated", OK: true}
	if got != want {
		t.Fatalf("got %+v; want %+v", got, want)
	}
	if !strings.Contains(stderr, "updated: 0.7.17 → 0.7.18") {
		t.Fatalf("the log is not on stderr:\n%s", stderr)
	}
}

func TestUpdateCLIOnlyJSONReportsAFailure(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)
	stubLatestCLI(t, "", errors.New("no route to host"))

	stdout, _, err := executeSplit(t, "--config", w.dir, "--format", "json", "update", "--cli-only")
	if err == nil {
		t.Fatal("a failure must still fail the command")
	}
	got := decodeCLIOnly(t, stdout)
	if got.OK || got.Status != "failed" || !strings.Contains(got.Error, "no route to host") || got.From != "0.7.18" {
		t.Fatalf("got %+v", got)
	}
}

func TestUpdateCLIOnlyJSONNamesHomebrew(t *testing.T) {
	w := newUpdateWorld(t, "v16", "v17", 0)
	root := t.TempDir()
	prefix := filepath.Join(root, "brew")
	binary := filepath.Join(prefix, "Cellar", "devmachine", "0.7.18", "bin", "devmachine")
	writeCommandFile(t, binary, "binary")
	fake := filepath.Join(root, "bin", "brew")
	writeCommandFile(t, fake, fmt.Sprintf("#!/bin/sh\n[ \"$1\" = --prefix ] && echo %q\nexit 0\n", prefix))
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(swap(&executablePath, func() (string, error) { return binary, nil }))
	t.Cleanup(swap(&findBrew, func() (string, error) { return fake, nil }))

	stdout, stderr, err := executeSplit(t, "--config", w.dir, "--format", "json", "update", "--cli-only")
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	got := decodeCLIOnly(t, stdout)
	want := cliOnlyJSON{From: "0.7.18", To: "0.7.18", Method: "homebrew", Status: "already latest", OK: true}
	if got != want {
		t.Fatalf("got %+v; want %+v", got, want)
	}
}

func TestUpdateWithoutCLIOnlyStillRefusesJSON(t *testing.T) {
	_, err := execute(t, "--format", "json", "update", "--skip-cli")
	if err == nil || !strings.Contains(err.Error(), "--cli-only") {
		t.Fatalf("the refusal does not point at --cli-only: %v", err)
	}
}
