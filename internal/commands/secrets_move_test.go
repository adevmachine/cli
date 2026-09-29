package commands

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/secrets"
)

// shellRemote runs what the CLI sends to a machine in a local shell, against
// a temporary home that getent hands out for every account.
type shellRemote struct {
	path string
}

func newShellRemote(t *testing.T, home string) *shellRemote {
	t.Helper()
	bin := t.TempDir()
	getent := "#!/bin/sh\nprintf '%s:x:0:0::" + home + ":/bin/sh\\n' \"$3\"\n"
	if err := os.WriteFile(filepath.Join(bin, "getent"), []byte(getent), 0o755); err != nil {
		t.Fatal(err)
	}
	return &shellRemote{path: bin + string(os.PathListSeparator) + os.Getenv("PATH")}
}

func (r *shellRemote) Run(ctx context.Context, command string) (string, error) {
	return r.RunInput(ctx, command, strings.NewReader(""))
}

func (r *shellRemote) RunInput(ctx context.Context, command string, stdin io.Reader) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(), "PATH="+r.path)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), &shellError{err: err, stderr: stderr.String()}
	}
	return stdout.String(), nil
}

func (r *shellRemote) Stream(ctx context.Context, command string, _, _ io.Writer) error {
	_, err := r.Run(ctx, command)
	return err
}

func (r *shellRemote) Upload(context.Context, string, io.Reader) error { return nil }
func (r *shellRemote) Close() error                                    { return nil }

type shellError struct {
	err    error
	stderr string
}

func (e *shellError) Error() string { return e.err.Error() + ": " + e.stderr }

// workspaceOnAShell is a workspace whose account is the one running the
// tests, so the delivery's chown works, and whose home is a temporary
// directory.
func workspaceOnAShell(t *testing.T) (dir, home string) {
	t.Helper()
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	dir = configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"+
		"workspaces:\n  - name: alice\n    user: "+me.Username+"\n")
	t.Cleanup(func() { _ = secrets.Delete(dir, "alice/API_KEY") })
	home, err = filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dialing(t, newShellRemote(t, home))
	return dir, home
}

func writeHomeFile(t *testing.T, home, rel, body string) {
	t.Helper()
	path := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readHomeFile(t *testing.T, home, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(home, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestCredentialsPushTakesAMovedSecretOutOfTheDefaultFile(t *testing.T) {
	dir, home := workspaceOnAShell(t)
	writeHomeFile(t, home, ".devmachine/env", "OTHER='kept'\nAPI_KEY='old'\n")
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "old", "--workspace", "alice")
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "new", "--workspace", "alice", "--env-file", "app/.env")

	out, err := execute(t, "--config", dir, "credentials", "push", "--yes")
	if err != nil {
		t.Fatalf("credentials push returned %v", err)
	}
	if got := readHomeFile(t, home, ".devmachine/env"); got != "OTHER='kept'\n" {
		t.Fatalf("the old file still has it: %q", got)
	}
	if got := readHomeFile(t, home, "app/.env"); got != "API_KEY='new'\n" {
		t.Fatalf("the new file has %q", got)
	}
	if !strings.Contains(out, ".devmachine/env") {
		t.Fatalf("the report does not say it left the old file: %q", out)
	}
	target, _, err := secrets.FindTarget(dir, "alice", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(target.LeftEnvFiles) != 0 {
		t.Fatalf("the old file is still remembered after the push: %#v", target)
	}
}

func TestCredentialsPushTakesAMovedSecretOutOfTheOtherEnvFile(t *testing.T) {
	dir, home := workspaceOnAShell(t)
	writeHomeFile(t, home, "app/.env", "API_KEY='old'\nDEBUG=1\n")
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "old", "--workspace", "alice", "--env-file", "app/.env")
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "new", "--workspace", "alice", "--env-file", "web/.env")

	if _, err := execute(t, "--config", dir, "credentials", "push", "--yes"); err != nil {
		t.Fatalf("credentials push returned %v", err)
	}
	if got := readHomeFile(t, home, "app/.env"); got != "DEBUG=1\n" {
		t.Fatalf("the old file still has it: %q", got)
	}
	if got := readHomeFile(t, home, "web/.env"); got != "API_KEY='new'\n" {
		t.Fatalf("the new file has %q", got)
	}
}

func TestCredentialsPushCheckLeavesAMovedSecretWhereItWas(t *testing.T) {
	dir, home := workspaceOnAShell(t)
	writeHomeFile(t, home, ".devmachine/env", "API_KEY='old'\n")
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "old", "--workspace", "alice")
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "new", "--workspace", "alice", "--env-file", "app/.env")

	out, err := execute(t, "--config", dir, "credentials", "push", "--check")
	if err != nil {
		t.Fatalf("credentials push --check returned %v", err)
	}
	if got := readHomeFile(t, home, ".devmachine/env"); got != "API_KEY='old'\n" {
		t.Fatalf("--check changed the old file: %q", got)
	}
	if !strings.Contains(out, "remove from .devmachine/env") {
		t.Fatalf("a dry run should say it would leave the old file: %q", out)
	}
	target, _, _ := secrets.FindTarget(dir, "alice", "API_KEY")
	if len(target.LeftEnvFiles) != 1 {
		t.Fatalf("--check forgot the old file: %#v", target)
	}
}

func TestSecretsSetPushTakesAMovedSecretOutOfTheFileItLeft(t *testing.T) {
	dir, home := workspaceOnAShell(t)
	writeHomeFile(t, home, ".devmachine/env", "API_KEY='old'\n")
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "old", "--workspace", "alice")

	if _, err := execute(t, "--config", dir, "secrets", "set", "API_KEY", "new",
		"--workspace", "alice", "--env-file", "app/.env", "--push"); err != nil {
		t.Fatalf("secrets set --push returned %v", err)
	}
	if got := readHomeFile(t, home, ".devmachine/env"); got != "" {
		t.Fatalf("the old file still has it: %q", got)
	}
	if got := readHomeFile(t, home, "app/.env"); got != "API_KEY='new'\n" {
		t.Fatalf("the new file has %q", got)
	}
	target, _, _ := secrets.FindTarget(dir, "alice", "API_KEY")
	if len(target.LeftEnvFiles) != 0 {
		t.Fatalf("the old file is still remembered: %#v", target)
	}
}

func TestCredentialsPushRemovalAlsoClearsTheFileASecretLeft(t *testing.T) {
	dir, home := workspaceOnAShell(t)
	writeHomeFile(t, home, ".devmachine/env", "API_KEY='old'\n")
	writeHomeFile(t, home, "app/.env", "API_KEY='new'\n")
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "old", "--workspace", "alice")
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "new", "--workspace", "alice", "--env-file", "app/.env")
	execute(t, "--config", dir, "secrets", "rm", "API_KEY", "--workspace", "alice", "--from-file")

	if _, err := execute(t, "--config", dir, "credentials", "push", "--yes"); err != nil {
		t.Fatalf("credentials push returned %v", err)
	}
	for _, rel := range []string{".devmachine/env", "app/.env"} {
		if got := readHomeFile(t, home, rel); got != "" {
			t.Fatalf("%s still has it: %q", rel, got)
		}
	}
}
