package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
	"github.com/adevmachine/cli/internal/secrets"
)

// recordingRemote answers like fakeRemote and keeps every command it was
// given, so a test can see what would have been written to the machine.
type recordingRemote struct {
	out  string
	err  error
	ran  []string
	sent []string
}

func (r *recordingRemote) Run(_ context.Context, command string) (string, error) {
	r.ran = append(r.ran, command)
	return r.out, r.err
}

func (r *recordingRemote) RunInput(_ context.Context, command string, stdin io.Reader) (string, error) {
	r.ran = append(r.ran, command)
	if stdin != nil {
		body, _ := io.ReadAll(stdin)
		r.sent = append(r.sent, string(body))
	}
	return r.out, r.err
}

func (r *recordingRemote) Stream(_ context.Context, command string, stdout, _ io.Writer) error {
	r.ran = append(r.ran, command)
	_, err := io.WriteString(stdout, r.out)
	if err != nil {
		return err
	}
	return r.err
}

func (r *recordingRemote) Upload(context.Context, string, io.Reader) error { return nil }

func (r *recordingRemote) Close() error { return nil }

func writeCredentialPackage(t *testing.T, configDir, name, scope, declared string) {
	t.Helper()
	dir := filepath.Join(packages.LocalDir(configDir), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("format: 1\nname: %s\nscope: %s\nsummary: A package written by a test.\ncredentials:\n%s",
		name, scope, declared)
	if err := os.WriteFile(filepath.Join(dir, packages.FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// configWithCredentials is a machine with a login of its own, a workspace with
// a login of its own, and a secret named by the caller.
//
// The secret's name is a parameter because a secret lives in the operating
// system's keychain, which is shared by every test in this package: two tests
// using one name would answer each other's question.
func configWithCredentials(t *testing.T, secret string) string {
	t.Helper()
	dir := configWith(t, fmt.Sprintf(`
machines:
  - name: main
    hosts: [203.0.113.10]
    packages: [gh-login]
workspaces:
  - name: alice
    packages: [claude-code, %s]
`, secret))

	writeCredentialPackage(t, dir, "gh-login", packages.ScopeMachine,
		"  - name: gh\n    kind: login\n    scope: machine\n"+
			"    command: gh auth login\n    stored_at: ~/.config/gh/hosts.yml\n")
	writeCredentialPackage(t, dir, "claude-code", packages.ScopeWorkspace,
		"  - name: claude\n    kind: login\n    scope: workspace\n"+
			"    command: claude /login\n    stored_at: ~/.claude/.credentials.json\n")
	writeCredentialPackage(t, dir, secret, packages.ScopeWorkspace,
		fmt.Sprintf("  - name: %s\n    kind: secret\n    scope: workspace\n    env: PROBE_TOKEN\n", secret))
	return dir
}

// answering makes the machine say which credentials are already there.
func answering(t *testing.T, lines ...string) *recordingRemote {
	t.Helper()
	client := &recordingRemote{out: strings.Join(lines, "\n") + "\n"}
	dialing(t, client)
	return client
}

// storeSecret puts a value in the store the CLI reads, and takes it out again
// afterwards so it does not outlive the test in the operator's keychain.
func storeSecret(t *testing.T, dir, name, value string) {
	t.Helper()
	if err := secrets.Set(dir, name, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secrets.Delete(dir, name) })
}

func TestCredentialsListSaysHowToFixAMissingLogin(t *testing.T) {
	dir := configWithCredentials(t, "probe-login")
	answering(t, "gh\tno", "alice/claude\tno", "alice/probe-login\tno")

	out, err := execute(t, "--config", dir, "credentials", "list")
	if err != nil {
		t.Fatalf("credentials list returned %v", err)
	}
	for _, want := range []string{"devmachine login gh", "devmachine login claude --workspace alice"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the report does not say how to fix it (%q):\n%s", want, out)
		}
	}
}

func TestCredentialsListSaysToStoreAMissingSecretFirst(t *testing.T) {
	dir := configWithCredentials(t, "probe-unstored")
	answering(t, "gh\tyes", "alice/claude\tyes", "alice/probe-unstored\tno")

	out, err := execute(t, "--config", dir, "credentials", "list")
	if err != nil {
		t.Fatalf("credentials list returned %v", err)
	}
	if !strings.Contains(out, "devmachine secrets set probe-unstored") {
		t.Fatalf("a secret nobody stored should say to store it:\n%s", out)
	}
	if !strings.Contains(out, "credentials push") {
		t.Fatalf("it does not say what delivers it:\n%s", out)
	}
}

func TestCredentialsListAsksOnlyForAPushWhenTheSecretIsStored(t *testing.T) {
	dir := configWithCredentials(t, "probe-stored")
	storeSecret(t, dir, "probe-stored", "value")
	answering(t, "gh\tyes", "alice/claude\tyes", "alice/probe-stored\tno")

	out, err := execute(t, "--config", dir, "credentials", "list")
	if err != nil {
		t.Fatalf("credentials list returned %v", err)
	}
	if strings.Contains(out, "secrets set probe-stored") {
		t.Fatalf("it asked for a secret that is already stored:\n%s", out)
	}
	if !strings.Contains(out, "devmachine credentials push") {
		t.Fatalf("it does not say what delivers it:\n%s", out)
	}
}

func TestCredentialsListNeverPrintsAValue(t *testing.T) {
	dir := configWithCredentials(t, "probe-quiet")
	storeSecret(t, dir, "probe-quiet", "s3cret-value")
	answering(t, "gh\tyes", "alice/claude\tyes", "alice/probe-quiet\tno")

	out, err := execute(t, "--config", dir, "credentials", "list")
	if err != nil {
		t.Fatalf("credentials list returned %v", err)
	}
	if strings.Contains(out, "s3cret-value") {
		t.Fatalf("the report carried a value:\n%s", out)
	}
}

func TestCredentialsListSaysWhatIsAlreadyThere(t *testing.T) {
	dir := configWithCredentials(t, "probe-present")
	answering(t, "gh\tyes", "alice/claude\tno", "alice/probe-present\tno")

	out, _ := execute(t, "--config", dir, "credentials", "list")
	line := lineWith(t, out, "gh ")
	if !strings.Contains(line, "present") {
		t.Fatalf("gh is on the machine and the report does not say so: %q", line)
	}
}

func TestCredentialsListAsJSONCarriesTheFixAndNoValue(t *testing.T) {
	dir := configWithCredentials(t, "probe-json")
	storeSecret(t, dir, "probe-json", "s3cret-value")
	answering(t, "gh\tno", "alice/claude\tno", "alice/probe-json\tno")

	out, err := execute(t, "--config", dir, "--format", "json", "credentials", "list")
	if err != nil {
		t.Fatalf("credentials list returned %v", err)
	}
	var got struct {
		Credentials []credentialJSON `json:"credentials"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output was not JSON: %v (%q)", err, out)
	}
	if len(got.Credentials) != 3 {
		t.Fatalf("got %#v", got.Credentials)
	}
	for _, c := range got.Credentials {
		if c.Status != "missing" || c.Fix == "" {
			t.Fatalf("every row should be missing and say what to do: %#v", c)
		}
	}
	if strings.Contains(out, "s3cret-value") {
		t.Fatalf("the JSON carried a value:\n%s", out)
	}
}

func TestCredentialsListSaysWhenNothingDeclaresACredential(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")
	answering(t, "")

	out, err := execute(t, "--config", dir, "credentials", "list")
	if err != nil {
		t.Fatalf("credentials list returned %v", err)
	}
	if !strings.Contains(out, "no credentials") {
		t.Fatalf("a machine with nothing declared should say so: %q", out)
	}
}

// lineWith returns the one output line holding want.
func lineWith(t *testing.T, out, want string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	t.Fatalf("no line holds %q:\n%s", want, out)
	return ""
}

var _ remote.Client = (*recordingRemote)(nil)
