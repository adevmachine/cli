package expose

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stubMachine is a sites directory and a PATH whose caddy, systemctl and
// chown only write down how they were called, so the real script runs here
// without touching anything outside a temporary directory.
type stubMachine struct {
	sites string
	bin   string
	calls string
}

func newStubMachine(t *testing.T) stubMachine {
	t.Helper()
	root := t.TempDir()
	m := stubMachine{
		sites: filepath.Join(root, "sites.d"),
		bin:   filepath.Join(root, "bin"),
		calls: filepath.Join(root, "calls"),
	}
	for _, dir := range []string{m.sites, m.bin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	m.stub(t, "caddy", `if [ "$1" = validate ] && [ -f "$STUB_ROOT/invalid" ]; then
  echo "Error: adapting config using caddyfile: ambiguous site definition: app.example.com"
  exit 1
fi
if [ "$1" = validate ]; then ls "$STUB_ROOT/sites.d" > "$STUB_ROOT/seen-by-validate"; fi
if [ "$1" = reload ] && [ -f "$STUB_ROOT/reload-fails" ]; then echo "caddy reload refused"; exit 1; fi`)
	m.stub(t, "systemctl", `if [ -f "$STUB_ROOT/reload-fails" ]; then echo "Job for caddy.service failed" >&2; exit 1; fi`)
	m.stub(t, "chown", "")
	return m
}

func (m stubMachine) stub(t *testing.T, name, body string) {
	t.Helper()
	script := "#!/bin/sh\necho \"" + name + " $*\" >> \"$STUB_ROOT/calls\"\n" + body + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(m.bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (m stubMachine) flag(t *testing.T, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(filepath.Dir(m.sites), name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (m stubMachine) write(t *testing.T, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(m.sites, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (m stubMachine) read(t *testing.T, name string) (string, bool) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(m.sites, name))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(body), true
}

func (m stubMachine) run(t *testing.T, change FileChange) (Outcome, string) {
	t.Helper()
	cmd := exec.Command("sh", "-s")
	cmd.Stdin = strings.NewReader(ApplyScript(change, filepath.Join(filepath.Dir(m.sites), "Caddyfile")))
	cmd.Env = append(os.Environ(), "PATH="+m.bin+":"+os.Getenv("PATH"), "STUB_ROOT="+filepath.Dir(m.sites))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the script failed: %v\n%s", err, out)
	}
	outcome, detail, err := ParseApply(string(out))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return outcome, detail
}

func (m stubMachine) callLog(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(m.calls)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(body)
}

func (m stubMachine) change(content string, stale ...string) FileChange {
	c := FileChange{Path: filepath.Join(m.sites, "alice-routes.caddy"), Owner: "root", Group: "root", Mode: "0644"}
	if content != "" {
		c.Content = []byte(content)
	}
	for _, s := range stale {
		c.Stale = append(c.Stale, filepath.Join(m.sites, s))
	}
	return c
}

func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || strings.Contains(e.Name(), ".devmachine-") {
			names = append(names, e.Name())
		}
	}
	return names
}

const aliceRoutes = "# workspace: alice\n\napp.example.com {\n\treverse_proxy 127.0.0.1:8080\n}\n"

func TestApplyWritesValidatesWithTheNewFileInPlaceThenReloads(t *testing.T) {
	m := newStubMachine(t)
	outcome, _ := m.run(t, m.change(aliceRoutes))
	if outcome != Applied {
		t.Fatalf("got %v", outcome)
	}
	if body, _ := m.read(t, "alice-routes.caddy"); body != aliceRoutes {
		t.Fatalf("got %q", body)
	}
	seen, _ := os.ReadFile(filepath.Join(filepath.Dir(m.sites), "seen-by-validate"))
	if !strings.Contains(string(seen), "alice-routes.caddy") {
		t.Fatalf("caddy validated without the new file in place: %q", seen)
	}
	calls := m.callLog(t)
	validate := strings.Index(calls, "caddy validate --config")
	reload := strings.Index(calls, "systemctl reload caddy")
	if validate < 0 || reload < validate {
		t.Fatalf("want validate, then reload:\n%s", calls)
	}
	if !strings.Contains(calls, "chown root:root") {
		t.Fatalf("the file was not given sync's owner:\n%s", calls)
	}
	if left := leftovers(t, m.sites); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

func TestApplyGivesTheFileSyncsMode(t *testing.T) {
	m := newStubMachine(t)
	m.run(t, m.change(aliceRoutes))
	info, err := os.Stat(filepath.Join(m.sites, "alice-routes.caddy"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestApplyThatCaddyRefusesKeepsTheOldFile(t *testing.T) {
	m := newStubMachine(t)
	m.write(t, "alice-routes.caddy", "old\n")
	m.write(t, "app.example.com.caddy", "legacy\n")
	m.flag(t, "invalid")

	outcome, detail := m.run(t, m.change(aliceRoutes, "app.example.com.caddy"))
	if outcome != Invalid {
		t.Fatalf("got %v", outcome)
	}
	if !strings.Contains(detail, "ambiguous site definition") {
		t.Fatalf("Caddy's own words are lost: %q", detail)
	}
	if body, _ := m.read(t, "alice-routes.caddy"); body != "old\n" {
		t.Fatalf("the old file was not kept: %q", body)
	}
	if body, _ := m.read(t, "app.example.com.caddy"); body != "legacy\n" {
		t.Fatalf("the legacy file was not put back: %q", body)
	}
	if strings.Contains(m.callLog(t), "systemctl") {
		t.Fatal("it reloaded a configuration Caddy refused")
	}
	if left := leftovers(t, m.sites); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

func TestApplyThatCannotReloadPutsTheOldFileBack(t *testing.T) {
	m := newStubMachine(t)
	m.write(t, "alice-routes.caddy", "old\n")
	m.flag(t, "reload-fails")

	outcome, detail := m.run(t, m.change(aliceRoutes))
	if outcome != ReloadFailed {
		t.Fatalf("got %v", outcome)
	}
	if !strings.Contains(detail, "caddy reload refused") {
		t.Fatalf("got %q", detail)
	}
	if !strings.Contains(m.callLog(t), "caddy reload --config") {
		t.Fatal("no fallback to caddy reload")
	}
	if body, _ := m.read(t, "alice-routes.caddy"); body != "old\n" {
		t.Fatalf("got %q", body)
	}
}

func TestApplyRemovesTheOldOneHostFiles(t *testing.T) {
	m := newStubMachine(t)
	m.write(t, "app.example.com.caddy", "legacy\n")
	if outcome, _ := m.run(t, m.change(aliceRoutes, "app.example.com.caddy")); outcome != Applied {
		t.Fatalf("got %v", outcome)
	}
	if _, ok := m.read(t, "app.example.com.caddy"); ok {
		t.Fatal("the legacy file is still there")
	}
}

func TestApplyWithNoContentRemovesTheFile(t *testing.T) {
	m := newStubMachine(t)
	m.write(t, "alice-routes.caddy", aliceRoutes)
	if outcome, _ := m.run(t, m.change("")); outcome != Applied {
		t.Fatalf("got %v", outcome)
	}
	if _, ok := m.read(t, "alice-routes.caddy"); ok {
		t.Fatal("the file is still there")
	}
	if !strings.Contains(m.callLog(t), "systemctl reload caddy") {
		t.Fatal("Caddy was not told")
	}
}

func TestApplyOfWhatIsAlreadyThereTouchesNothing(t *testing.T) {
	m := newStubMachine(t)
	m.write(t, "alice-routes.caddy", aliceRoutes)
	if outcome, _ := m.run(t, m.change(aliceRoutes)); outcome != Unchanged {
		t.Fatalf("got %v", outcome)
	}
	if strings.Contains(m.callLog(t), "systemctl") {
		t.Fatal("it reloaded for nothing")
	}
	if left := leftovers(t, m.sites); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

func TestApplyWithoutCaddySaysSoAndWritesNothing(t *testing.T) {
	m := newStubMachine(t)
	if err := os.RemoveAll(m.sites); err != nil {
		t.Fatal(err)
	}
	if outcome, _ := m.run(t, m.change(aliceRoutes)); outcome != NoCaddy {
		t.Fatalf("got %v", outcome)
	}
}

func TestApplyScriptQuotesEveryPath(t *testing.T) {
	c := FileChange{Path: "/etc/caddy/sites.d/o'brien-routes.caddy", Content: []byte("x"),
		Owner: "root", Group: "root", Mode: "0644"}
	script := ApplyScript(c, "/etc/caddy/Caddyfile")
	if !strings.Contains(script, `'/etc/caddy/sites.d/o'\''brien-routes.caddy'`) {
		t.Fatalf("the path is not quoted:\n%s", script)
	}
	if strings.Contains(script, "x\n") {
		t.Fatal("the content must travel encoded, never as shell text")
	}
}

func TestApplyCommandBecomesRootOnlyWhenItIsNotAlready(t *testing.T) {
	if !strings.Contains(ApplyCommand, "id -u") || !strings.Contains(ApplyCommand, "sudo -n sh -s") {
		t.Fatalf("got %q", ApplyCommand)
	}
}

func TestParseApplyRefusesOutputWithNoVerdict(t *testing.T) {
	if _, _, err := ParseApply("something else\n"); err == nil {
		t.Fatal("an answer with no verdict must not read as success")
	}
}

func TestDiffMarksWhatChanged(t *testing.T) {
	got := Diff("a\nb\nc\n", "a\nc\nd\n")
	want := " a\n-b\n c\n+d\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}
