package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/remote"
)

// homeRemote runs what upload sends on this computer, with HOME pointing at
// a temporary folder for that child process alone.
type homeRemote struct{ home string }

func (c homeRemote) Run(ctx context.Context, command string) (string, error) {
	return c.RunInput(ctx, command, strings.NewReader(""))
}

func (c homeRemote) RunInput(ctx context.Context, command string, stdin io.Reader) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(), "HOME="+c.home)
	cmd.Stdin = stdin
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.String(), err
}

func (c homeRemote) Stream(ctx context.Context, command string, _, _ io.Writer) error {
	_, err := c.Run(ctx, command)
	return err
}

func (c homeRemote) Upload(context.Context, string, io.Reader) error { return nil }
func (c homeRemote) Close() error                                    { return nil }

// uploadingTo answers every dial with a homeRemote on a fresh home, fixes
// the clock, and returns the home and the dials made.
func uploadingTo(t *testing.T) (string, *[]dialCall) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var calls []dialCall
	stub := func(_ context.Context, m config.Machine, user string) (remote.Client, string, error) {
		calls = append(calls, dialCall{machine: m.Name, user: user})
		return homeRemote{home: home}, "203.0.113.10", nil
	}
	origDial, origDialMux := dial, dialMux
	dial, dialMux = stub, stub
	uploadNow = func() time.Time { return time.Date(2026, 9, 30, 14, 30, 12, 0, time.Local) }
	t.Cleanup(func() { dial, dialMux, uploadNow = origDial, origDialMux, time.Now })
	return home, &calls
}

func localFile(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const oneMachineConfig = "machines:\n  - name: main\n    hosts: [203.0.113.10]\nworkspaces:\n  - name: acme\n    machine: main\n"

func TestUploadSendsIntoTheWorkspaceAsItsOwnAccount(t *testing.T) {
	home, calls := uploadingTo(t)
	dir := configWith(t, twoMachineConfig)
	file := localFile(t, "report.pdf", "pdf body")

	out, stderr, err := executeSplit(t, "--config", dir, "upload", file, "--workspace", "bob")
	if err != nil {
		t.Fatalf("upload returned %v (%s)", err, stderr)
	}
	want := filepath.Join(home, ".cache", "devmachine", "uploads", "report-20260930-143012.pdf")
	if out != want+"\n" {
		t.Fatalf("stdout %q, want only %q", out, want)
	}
	if body, _ := os.ReadFile(want); string(body) != "pdf body" {
		t.Fatalf("body %q", body)
	}
	if len(*calls) != 1 || (*calls)[0] != (dialCall{machine: "sandbox", user: "bob-dev"}) {
		t.Fatalf("dialed %#v, want sandbox as bob-dev", *calls)
	}
}

func TestUploadWithMachineSendsAsTheAdmin(t *testing.T) {
	_, calls := uploadingTo(t)
	dir := configWith(t, twoMachineConfig)

	if _, err := execute(t, "--config", dir, "--machine", "sandbox", "upload", localFile(t, "a.txt", "x")); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != (dialCall{machine: "sandbox", user: ""}) {
		t.Fatalf("dialed %#v, want sandbox as its admin", *calls)
	}
}

func TestUploadWithOneMachineNeedsNoFlag(t *testing.T) {
	_, calls := uploadingTo(t)
	dir := configWith(t, oneMachineConfig)

	if _, err := execute(t, "--config", dir, "upload", localFile(t, "a.txt", "x")); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != (dialCall{machine: "main", user: ""}) {
		t.Fatalf("dialed %#v, want main as its admin", *calls)
	}
}

func TestUploadNeverGuessesBetweenSeveralMachines(t *testing.T) {
	_, calls := uploadingTo(t)
	dir := configWith(t, twoMachineConfig)

	_, err := execute(t, "--config", dir, "upload", localFile(t, "a.txt", "x"))
	if err == nil || !strings.Contains(err.Error(), "--machine") {
		t.Fatalf("got %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("it dialed a machine nobody named: %#v", *calls)
	}
}

func TestUploadRefusesAWorkspaceOnAnotherMachineThanTheOneNamed(t *testing.T) {
	_, calls := uploadingTo(t)
	dir := configWith(t, twoMachineConfig)

	_, err := execute(t, "--config", dir, "--machine", "main", "upload", localFile(t, "a.txt", "x"), "--workspace", "bob")
	if err == nil || !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("got %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("it dialed anyway: %#v", *calls)
	}
}

func TestUploadRefusesYourOwnComputer(t *testing.T) {
	_, calls := uploadingTo(t)
	dir := configWith(t, "machines:\n  - name: laptop\n    self: true\n")

	_, err := execute(t, "--config", dir, "upload", localFile(t, "a.txt", "x"))
	if err == nil || !strings.Contains(err.Error(), "self: true") {
		t.Fatalf("got %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("it dialed anyway: %#v", *calls)
	}
}

func TestUploadRefusesAFolderBeforeConnecting(t *testing.T) {
	_, calls := uploadingTo(t)
	dir := configWith(t, oneMachineConfig)

	_, err := execute(t, "--config", dir, "upload", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "upload sends files") || !strings.Contains(err.Error(), "tar") {
		t.Fatalf("got %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("it dialed for nothing to send: %#v", *calls)
	}
}

func TestUploadRefusesAMissingFileBeforeConnecting(t *testing.T) {
	_, calls := uploadingTo(t)
	dir := configWith(t, oneMachineConfig)

	_, err := execute(t, "--config", dir, "upload", filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil || !strings.Contains(err.Error(), "nope.txt") {
		t.Fatalf("got %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("it dialed for nothing to send: %#v", *calls)
	}
}

func TestUploadRefusesADirThatClimbsOutOfTheHomeBeforeConnecting(t *testing.T) {
	_, calls := uploadingTo(t)
	dir := configWith(t, oneMachineConfig)

	_, err := execute(t, "--config", dir, "upload", localFile(t, "a.txt", "x"), "--workspace", "acme", "--dir", "../bob")
	if err == nil || !strings.Contains(err.Error(), "outside the home") {
		t.Fatalf("got %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("it dialed anyway: %#v", *calls)
	}
}

func TestUploadPrintsJSONWithLocalRemoteAndBytes(t *testing.T) {
	home, _ := uploadingTo(t)
	dir := configWith(t, oneMachineConfig)
	a := localFile(t, "a.txt", "four")
	b := localFile(t, "relatório final.md", "seven!!")

	out, _, err := executeSplit(t, "--config", dir, "--format", "json", "upload", a, b, "--workspace", "acme", "--dir", "notes")
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	want := []map[string]any{
		{"local": a, "remote": filepath.Join(home, "notes", "a-20260930-143012.txt"), "bytes": float64(4)},
		{"local": b, "remote": filepath.Join(home, "notes", "relatório final-20260930-143012.md"), "bytes": float64(7)},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("entry %d has fields %v, want %v", i, got[i], want[i])
		}
		for k, v := range want[i] {
			if got[i][k] != v {
				t.Fatalf("entry %d %s = %v, want %v", i, k, got[i][k], v)
			}
		}
	}
}

func TestUploadTriesEveryFileAndFailsIfAnyFailed(t *testing.T) {
	home, _ := uploadingTo(t)
	dir := configWith(t, oneMachineConfig)
	good := localFile(t, "good.txt", "ok")
	missing := filepath.Join(t.TempDir(), "missing.txt")

	out, stderr, err := executeSplit(t, "--config", dir, "upload", missing, good)
	if err == nil {
		t.Fatal("expected a failure for the missing file")
	}
	uploaded := filepath.Join(home, ".cache", "devmachine", "uploads", "good-20260930-143012.txt")
	if out != uploaded+"\n" {
		t.Fatalf("stdout %q, want only the uploaded path", out)
	}
	if !strings.Contains(stderr, "missing.txt") {
		t.Fatalf("stderr does not name the failed file: %q", stderr)
	}
	if _, err := os.Stat(uploaded); err != nil {
		t.Fatalf("the good file was not sent: %v", err)
	}
}

func TestUploadReportsAFailureInsideTheJSON(t *testing.T) {
	uploadingTo(t)
	dir := configWith(t, oneMachineConfig)
	missing := filepath.Join(t.TempDir(), "missing.txt")

	out, _, err := executeSplit(t, "--config", dir, "--format", "json", "upload", localFile(t, "a.txt", "x"), missing)
	if err == nil {
		t.Fatal("expected a failure for the missing file")
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got) != 2 || got[0]["error"] != nil || got[1]["error"] == nil {
		t.Fatalf("got %v", got)
	}
}

func TestUploadRecordsEachFileInTheCommandLog(t *testing.T) {
	uploadingTo(t)
	dir := configWith(t, oneMachineConfig)

	if _, err := execute(t, "--config", dir, "upload", localFile(t, "a.txt", "x"), "--workspace", "acme"); err != nil {
		t.Fatal(err)
	}
	line := historyLines(t, dir)[0]
	for _, want := range []string{"workspace acme", "upload", "a.txt", "ok"} {
		if !strings.Contains(line, want) {
			t.Fatalf("the log line leaves out %q: %q", want, line)
		}
	}
}
