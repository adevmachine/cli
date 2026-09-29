package credentials

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
)

// localClient runs a delivery's shell on this computer, against a temporary
// home, with getent answering for any account with that home.
type localClient struct {
	path string
}

func newLocalClient(t *testing.T, home string) *localClient {
	t.Helper()
	bin := t.TempDir()
	getent := "#!/bin/sh\nprintf '%s:x:0:0::" + home + ":/bin/sh\\n' \"$3\"\n"
	if err := os.WriteFile(filepath.Join(bin, "getent"), []byte(getent), 0o755); err != nil {
		t.Fatal(err)
	}
	return &localClient{path: bin + string(os.PathListSeparator) + os.Getenv("PATH")}
}

func (c *localClient) Run(ctx context.Context, command string) (string, error) {
	return c.RunInput(ctx, command, strings.NewReader(""))
}

func (c *localClient) RunInput(ctx context.Context, command string, stdin io.Reader) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(), "PATH="+c.path)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), &scriptError{err: err, stderr: stderr.String()}
	}
	return stdout.String(), nil
}

func (c *localClient) Stream(ctx context.Context, command string, _, _ io.Writer) error {
	_, err := c.Run(ctx, command)
	return err
}

func (c *localClient) Upload(context.Context, string, io.Reader) error { return nil }
func (c *localClient) Close() error                                    { return nil }

type scriptError struct {
	err    error
	stderr string
}

func (e *scriptError) Error() string { return e.err.Error() + ": " + e.stderr }

// homeAndOutside is a workspace home and, next to it, a directory the
// workspace must never reach, both resolved so a symlinked TMPDIR does not
// blur the line between them.
func homeAndOutside(t *testing.T) (home, outside string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home = filepath.Join(root, "home")
	outside = filepath.Join(root, "outside")
	for _, d := range []string{home, outside} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home, outside
}

func currentUser(t *testing.T) string {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	return u.Username
}

func TestPushWorkspaceEnvWritesAFileInsideTheHome(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := newLocalClient(t, home)
	if err := os.MkdirAll(filepath.Join(home, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "app", ".env"), []byte("KEEP=me\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := PushWorkspaceEnv(context.Background(), c, currentUser(t), "app/.env", "API_KEY", "v", true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(home, "app", ".env"))
	if string(got) != "KEEP=me\nAPI_KEY='v'\n" {
		t.Fatalf("got %q", got)
	}
	backup, _ := os.ReadFile(filepath.Join(home, "app", ".env"+backupSuffix))
	if string(backup) != "KEEP=me\n" {
		t.Fatalf("backup %q", backup)
	}
}

func TestPushWorkspaceEnvRefusesADirectoryThatLinksOutOfTheHome(t *testing.T) {
	home, outside := homeAndOutside(t)
	c := newLocalClient(t, home)
	if err := os.Symlink(outside, filepath.Join(home, "escape")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(outside)

	err := PushWorkspaceEnv(context.Background(), c, currentUser(t), "escape/leak.env", "API_KEY", "v", true)
	if err == nil || !strings.Contains(err.Error(), "outside the workspace's home") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "leak.env")); !os.IsNotExist(err) {
		t.Fatalf("it wrote outside the home: %v", err)
	}
	after, _ := os.Stat(outside)
	if after.Mode() != before.Mode() {
		t.Fatalf("it changed the mode outside the home: %v -> %v", before.Mode(), after.Mode())
	}
}

func TestPushWorkspaceEnvRefusesAFileThatIsASymlink(t *testing.T) {
	home, outside := homeAndOutside(t)
	c := newLocalClient(t, home)
	secret := filepath.Join(outside, "shadow")
	if err := os.WriteFile(secret, []byte("root:hash\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(home, ".env")); err != nil {
		t.Fatal(err)
	}

	err := PushWorkspaceEnv(context.Background(), c, currentUser(t), ".env", "API_KEY", "v", true)
	if err == nil || !strings.Contains(err.Error(), "outside the workspace's home") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".env"+backupSuffix)); !os.IsNotExist(err) {
		t.Fatalf("it copied the linked file into the home: %v", err)
	}
	link, _ := os.Readlink(filepath.Join(home, ".env"))
	if link != secret {
		t.Fatalf("the link was replaced: %q", link)
	}
}

func TestPushWorkspaceEnvRefusesABackupThatIsASymlink(t *testing.T) {
	home, outside := homeAndOutside(t)
	c := newLocalClient(t, home)
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(outside, "victim")
	if err := os.Symlink(victim, filepath.Join(home, ".env"+backupSuffix)); err != nil {
		t.Fatal(err)
	}

	err := PushWorkspaceEnv(context.Background(), c, currentUser(t), ".env", "API_KEY", "v", true)
	if err == nil || !strings.Contains(err.Error(), "outside the workspace's home") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(victim); !os.IsNotExist(err) {
		t.Fatalf("the backup was written through the link: %v", err)
	}
}

func TestPushWorkspaceEnvFollowsALinkThatStaysInsideTheHome(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := newLocalClient(t, home)
	if err := os.Mkdir(filepath.Join(home, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "real"), filepath.Join(home, "app")); err != nil {
		t.Fatal(err)
	}

	if err := PushWorkspaceEnv(context.Background(), c, currentUser(t), "app/.env", "API_KEY", "v", true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(home, "real", ".env"))
	if string(got) != "API_KEY='v'\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRemoveWorkspaceEnvRefusesADirectoryThatLinksOutOfTheHome(t *testing.T) {
	home, outside := homeAndOutside(t)
	c := newLocalClient(t, home)
	if err := os.WriteFile(filepath.Join(outside, ".env"), []byte("API_KEY=x\nOTHER=y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "escape")); err != nil {
		t.Fatal(err)
	}

	err := RemoveWorkspaceEnv(context.Background(), c, currentUser(t), "escape/.env", "API_KEY", true)
	if err == nil || !strings.Contains(err.Error(), "outside the workspace's home") {
		t.Fatalf("got %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(outside, ".env"))
	if string(got) != "API_KEY=x\nOTHER=y\n" {
		t.Fatalf("the file outside the home changed: %q", got)
	}
}
