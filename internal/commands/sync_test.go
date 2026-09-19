package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/provision"
	"github.com/adevmachine/cli/internal/remote"
)

// syncStub stands in for the thing that runs Ansible, so a sync can be driven
// end to end without a machine.
type syncStub struct {
	called  bool
	plan    packages.MachinePlan
	options provision.Options
	result  provision.Result
	err     error
}

func (s *syncStub) Apply(_ context.Context, plan packages.MachinePlan, opts provision.Options) (provision.Result, error) {
	s.called, s.plan, s.options = true, plan, opts
	return s.result, s.err
}

func stubSync(t *testing.T) *syncStub {
	t.Helper()
	stub := &syncStub{result: provision.Result{Ok: 4, Changed: 2}}
	dialing(t, fakeRemote{})
	provisionerFor = func(remote.Client) provision.Provisioner { return stub }
	t.Cleanup(func() { provisionerFor = ansibleProvisioner })
	return stub
}

// executeSplit keeps the two streams apart, which is the only way to see that
// JSON on stdout is not mixed with the run's own output.
func executeSplit(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	out, errs := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errs)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errs.String(), err
}

// executeWithInput runs the tree with something on standard input, which is
// what a confirmation reads.
func executeWithInput(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func configWithPackages(t *testing.T) string {
	t.Helper()
	dir := configWith(t, `
machines:
  - name: main
    hosts: [203.0.113.10]
    packages: [base]
workspaces:
  - name: alice
    packages: [claude-code]
`)
	writeLocalPackage(t, dir, "base", packages.ScopeMachine)
	writeLocalPackage(t, dir, "claude-code", packages.ScopeWorkspace)
	return dir
}

func configWithBrokenLocalPackage(t *testing.T) string {
	t.Helper()
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [base]\n")
	writeLocalPackage(t, dir, "base", packages.ScopeMachine)

	// A missing summary is something resolution does not look at, so only the
	// validator can catch it.
	manifest := filepath.Join(packages.LocalDir(dir), "base", "package.yml")
	if err := os.WriteFile(manifest, []byte("format: 1\nname: base\nscope: machine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSyncPrintsThePlanAndAsks(t *testing.T) {
	stub := stubSync(t)

	out, err := executeWithInput(t, "n\n", "--config", configWithPackages(t), "sync")
	if err == nil {
		t.Fatal("answering no still applied")
	}
	for _, want := range []string{"main", "base", "alice", "claude-code"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the plan leaves out %q: %s", want, out)
		}
	}
	if stub.called {
		t.Fatal("it applied after the answer was no")
	}
}

func TestSyncAppliesWhenTheAnswerIsYes(t *testing.T) {
	stub := stubSync(t)

	if _, err := executeWithInput(t, "y\n", "--config", configWithPackages(t), "sync"); err != nil {
		t.Fatal(err)
	}
	if !stub.called {
		t.Fatal("it did not apply")
	}
}

func TestSyncCheckNeverAsks(t *testing.T) {
	stub := stubSync(t)

	if _, err := execute(t, "--config", configWithPackages(t), "sync", "--check"); err != nil {
		t.Fatal(err)
	}
	if !stub.options.Check {
		t.Fatal("--check was not passed through")
	}
}

func TestSyncPassesTheTagsThrough(t *testing.T) {
	stub := stubSync(t)

	if _, err := execute(t, "--config", configWithPackages(t), "sync", "--yes", "--tags", "base,caddy"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(stub.options.Tags, ",") != "base,caddy" {
		t.Fatalf("the tags did not reach the run: %#v", stub.options.Tags)
	}
}

func TestSyncStreamsTheMachineOutputSomewhere(t *testing.T) {
	stub := stubSync(t)

	if _, err := execute(t, "--config", configWithPackages(t), "sync", "--yes"); err != nil {
		t.Fatal(err)
	}
	// A run collected and printed at the end looks stuck when it is not.
	if stub.options.Out == nil {
		t.Fatal("the run had nowhere to stream to")
	}
}

func TestSyncWritesTheLockOnSuccess(t *testing.T) {
	stubSync(t)
	dir := configWithPackages(t)

	if _, err := execute(t, "--config", dir, "sync", "--yes"); err != nil {
		t.Fatal(err)
	}

	lock, err := packages.LoadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Machines["main"]) == 0 {
		t.Fatal("the lock was not written")
	}
	if len(lock.Workspaces["alice"]) == 0 {
		t.Fatal("the workspace was left out of the lock")
	}
}

func TestSyncDoesNotWriteTheLockOnACheck(t *testing.T) {
	stubSync(t)
	dir := configWithPackages(t)

	if _, err := execute(t, "--config", dir, "sync", "--check"); err != nil {
		t.Fatal(err)
	}

	lock, err := packages.LoadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Machines) != 0 {
		t.Fatal("a dry run recorded something as applied")
	}
}

func TestSyncDoesNotWriteTheLockWhenTheRunFailed(t *testing.T) {
	stub := stubSync(t)
	stub.err = errors.New("exit status 2")
	stub.result = provision.Result{Ok: 1, Failed: 1}
	dir := configWithPackages(t)

	if _, err := execute(t, "--config", dir, "sync", "--yes"); err == nil {
		t.Fatal("a failed run reported success")
	}

	lock, _ := packages.LoadLock(dir)
	if len(lock.Machines) != 0 {
		t.Fatal("a failed run claimed the machine holds those packages")
	}
}

func TestSyncRefusesAnInvalidLocalPackageBeforeConnecting(t *testing.T) {
	stub := stubSync(t)
	dir := configWithBrokenLocalPackage(t)

	out, err := execute(t, "--config", dir, "sync", "--yes")
	if err == nil {
		t.Fatal("a broken local package was applied")
	}
	if !strings.Contains(out+err.Error(), "package.yml") {
		t.Fatalf("the error does not point at the file: %v %s", err, out)
	}
	if stub.called {
		t.Fatal("it reached the machine anyway")
	}
}

func TestSyncRecordsTheRunInTheLog(t *testing.T) {
	stubSync(t)
	dir := configWithPackages(t)

	if _, err := execute(t, "--config", dir, "sync", "--check", "--tags", "base"); err != nil {
		t.Fatal(err)
	}

	line := historyLines(t, dir)[0]
	// A dry run that told somebody the wrong thing is worth being able to find.
	for _, want := range []string{"machine main", "sync --check --tags base", "ok"} {
		if !strings.Contains(line, want) {
			t.Fatalf("the log line leaves out %q: %q", want, line)
		}
	}
}

func TestSyncAsJSONCarriesThePlanAndTheResult(t *testing.T) {
	stubSync(t)

	out, diagnostics, err := executeSplit(t, "--config", configWithPackages(t), "--format", "json", "sync", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diagnostics, "base") {
		t.Fatalf("the plan did not go to stderr: %q", diagnostics)
	}
	var got struct {
		Machine string   `json:"machine"`
		Check   bool     `json:"check"`
		Plan    []string `json:"plan"`
		Result  struct {
			Ok      int `json:"ok"`
			Changed int `json:"changed"`
			Failed  int `json:"failed"`
		} `json:"result"`
		Locked bool `json:"locked"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, out)
	}
	if got.Machine != "main" || len(got.Plan) == 0 || got.Result.Ok != 4 || !got.Locked {
		t.Fatalf("got %#v", got)
	}
}

func TestSyncNeedsAMachineWhenThereAreSeveral(t *testing.T) {
	stub := stubSync(t)
	dir := configWith(t, twoMachineConfig)

	_, err := execute(t, "--config", dir, "sync", "--yes")
	if err == nil {
		t.Fatal("it picked a machine nobody named")
	}
	if !strings.Contains(err.Error(), "--machine") {
		t.Fatalf("the error does not say how to choose: %v", err)
	}
	if stub.called {
		t.Fatal("it applied to a machine nobody named")
	}
}
