package upload

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// homeClient runs the upload's shell on this computer, as the account that
// owns home, the way ssh runs it as the account it logs in as. HOME is set
// for that child alone.
type homeClient struct {
	home     string
	commands []string
}

func (c *homeClient) Run(ctx context.Context, command string) (string, error) {
	return c.RunInput(ctx, command, strings.NewReader(""))
}

func (c *homeClient) RunInput(ctx context.Context, command string, stdin io.Reader) (string, error) {
	c.commands = append(c.commands, command)
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(), "HOME="+c.home)
	cmd.Stdin = stdin
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.String(), err
}

func (c *homeClient) Stream(ctx context.Context, command string, _, _ io.Writer) error {
	_, err := c.Run(ctx, command)
	return err
}

func (c *homeClient) Upload(context.Context, string, io.Reader) error { return nil }
func (c *homeClient) Close() error                                    { return nil }

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

func send(t *testing.T, c *homeClient, dir, name, body string) (string, error) {
	t.Helper()
	stem, ext := Name(name, at)
	return Send(context.Background(), c, Request{Dir: dir, Stem: stem, Ext: ext, Mode: "0600"}, strings.NewReader(body))
}

func TestSendWritesIntoTheDefaultFolderWithTheTimestampedName(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := &homeClient{home: home}
	dir, _ := Dir("")

	got, err := send(t, c, dir, "report.pdf", "pdf body")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".cache", "devmachine", "uploads", "report-20260930-143012.pdf")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	body, _ := os.ReadFile(want)
	if string(body) != "pdf body" {
		t.Fatalf("body %q", body)
	}
	info, _ := os.Stat(want)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	folder, _ := os.Stat(filepath.Dir(want))
	if folder.Mode().Perm() != 0o700 {
		t.Fatalf("folder mode %v", folder.Mode().Perm())
	}
}

func TestSendNumbersACollisionInsteadOfOverwriting(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := &homeClient{home: home}

	var paths []string
	for _, body := range []string{"one", "two", "three"} {
		got, err := send(t, c, "notes", "report.pdf", body)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filepath.Base(got))
	}
	want := []string{"report-20260930-143012.pdf", "report-20260930-143012-2.pdf", "report-20260930-143012-3.pdf"}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("got %v, want %v", paths, want)
		}
	}
	first, _ := os.ReadFile(filepath.Join(home, "notes", want[0]))
	if string(first) != "one" {
		t.Fatalf("the first file was overwritten: %q", first)
	}
	leftovers, _ := filepath.Glob(filepath.Join(home, "notes", ".devmachine-upload.*"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files were left behind: %v", leftovers)
	}
}

func TestSendKeepsSpacesAndUnicodeInTheName(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := &homeClient{home: home}

	got, err := send(t, c, "notes", "relatório final.txt", "x")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "relatório final-20260930-143012.txt" {
		t.Fatalf("got %q", got)
	}
}

func TestSendNeverPutsAHostileNameInTheCommand(t *testing.T) {
	home, outside := homeAndOutside(t)
	c := &homeClient{home: home}
	canary := filepath.Join(outside, "canary")
	if err := os.WriteFile(canary, []byte("alive"), 0o644); err != nil {
		t.Fatal(err)
	}
	hostile := "'; rm -rf " + outside + "; $(touch " + outside + "/pwned) `id` \".txt"

	got, err := send(t, c, "notes", hostile, "x")
	if err != nil {
		t.Fatal(err)
	}
	stem, ext := Name(hostile, at)
	if filepath.Base(got) != stem+ext {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(canary); err != nil {
		t.Fatalf("the name ran as a command: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned")); !os.IsNotExist(err) {
		t.Fatalf("the name ran as a command: %v", err)
	}
	for _, part := range []string{"rm -rf", "$(touch", "`id`"} {
		if strings.Contains(c.commands[0], part) {
			t.Fatalf("the command carries the raw name: %q", c.commands[0])
		}
	}
}

func TestTheScriptHasNoSingleQuoteSoItSurvivesAnyLoginShell(t *testing.T) {
	if strings.Contains(script, "'") {
		t.Fatal("the script has a single quote, which would end the quoting around it")
	}
}

func TestSendAcceptsAnAbsoluteFolderInsideTheHome(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := &homeClient{home: home}

	got, err := send(t, c, filepath.Join(home, "inbox"), "a.txt", "x")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(got) != filepath.Join(home, "inbox") {
		t.Fatalf("got %q", got)
	}
}

func TestSendRefusesAnAbsoluteFolderOutsideTheHome(t *testing.T) {
	home, outside := homeAndOutside(t)
	c := &homeClient{home: home}

	_, err := send(t, c, outside, "a.txt", "x")
	if err == nil || !strings.Contains(err.Error(), "outside the home") {
		t.Fatalf("got %v", err)
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatalf("it wrote outside the home: %v", entries)
	}
}

func TestSendRefusesAFolderThatLinksOutOfTheHome(t *testing.T) {
	home, outside := homeAndOutside(t)
	c := &homeClient{home: home}
	if err := os.Symlink(outside, filepath.Join(home, "escape")); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{"escape", "escape/deeper", filepath.Join(home, "escape")} {
		_, err := send(t, c, dir, "a.txt", "x")
		if err == nil || !strings.Contains(err.Error(), "outside the home") {
			t.Fatalf("%s: got %v", dir, err)
		}
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatalf("it wrote outside the home: %v", entries)
	}
}

func TestSendRefusesAFolderThatIsAFile(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := &homeClient{home: home}
	if err := os.WriteFile(filepath.Join(home, "notes"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := send(t, c, "notes", "a.txt", "x")
	if err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Fatalf("got %v", err)
	}
}

func TestSendAppliesTheRequestedMode(t *testing.T) {
	home, _ := homeAndOutside(t)
	c := &homeClient{home: home}
	stem, ext := Name("a.txt", at)

	got, err := Send(context.Background(), c, Request{Dir: ".", Stem: stem, Ext: ext, Mode: "0644"}, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(got)
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}
