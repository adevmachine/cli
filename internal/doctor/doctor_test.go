package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/remote"
)

type fakeClient struct {
	out map[string]string
	err map[string]error
}

func (f fakeClient) Run(_ context.Context, command string) (string, error) {
	if err, ok := f.err[command]; ok {
		return "", err
	}
	return f.out[command], nil
}

func (f fakeClient) Close() error { return nil }

func configDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func find(t *testing.T, checks []Check, name string) Check {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q in %#v", name, checks)
	return Check{}
}

func TestAMissingConfigFailsAndSkipsTheRest(t *testing.T) {
	dialled := false
	dial := func(context.Context, config.Config) (remote.Client, string, error) {
		dialled = true
		return nil, "", nil
	}

	checks := Run(context.Background(), configDir(t, ""), dial)

	if got := find(t, checks, CheckConfiguration); got.Status != StatusFail {
		t.Fatalf("configuration = %q, want fail", got.Status)
	}
	if got := find(t, checks, CheckConnection); got.Status != StatusSkip {
		t.Fatalf("connection = %q, want skip", got.Status)
	}
	if dialled {
		t.Fatal("it tried to connect with no usable configuration")
	}
}

func TestAConfigWithNoHostFailsBeforeDialling(t *testing.T) {
	dialled := false
	dial := func(context.Context, config.Config) (remote.Client, string, error) {
		dialled = true
		return nil, "", nil
	}

	checks := Run(context.Background(), configDir(t, "domain: example.com\n"), dial)

	if got := find(t, checks, CheckConfiguration); got.Status != StatusFail {
		t.Fatalf("configuration = %q, want fail", got.Status)
	}
	if dialled {
		t.Fatal("it tried to connect with an invalid configuration")
	}
}

func TestAnUnreachableMachineSkipsTheRemoteChecks(t *testing.T) {
	dial := func(context.Context, config.Config) (remote.Client, string, error) {
		return nil, "", errors.New("connection refused")
	}

	checks := Run(context.Background(), configDir(t, "hosts:\n  - 203.0.113.10\n"), dial)

	if got := find(t, checks, CheckConfiguration); got.Status != StatusPass {
		t.Fatalf("configuration = %q, want pass", got.Status)
	}
	if got := find(t, checks, CheckConnection); got.Status != StatusFail {
		t.Fatalf("connection = %q, want fail", got.Status)
	}
	// A cascade of failures hides the one that matters, so everything that
	// depended on the connection is skipped rather than failed.
	for _, name := range []string{CheckOperatingSystem, CheckAnsible} {
		if got := find(t, checks, name); got.Status != StatusSkip {
			t.Fatalf("%s = %q, want skip", name, got.Status)
		}
	}
}

func TestASupportedOperatingSystemPasses(t *testing.T) {
	client := fakeClient{out: map[string]string{
		osReleaseCommand: "ID=ubuntu\nID_LIKE=debian\n",
		ansibleCommand:   "/usr/bin/ansible-playbook\n",
	}}
	dial := func(context.Context, config.Config) (remote.Client, string, error) {
		return client, "203.0.113.10", nil
	}

	checks := Run(context.Background(), configDir(t, "hosts:\n  - 203.0.113.10\n"), dial)

	if got := find(t, checks, CheckOperatingSystem); got.Status != StatusPass {
		t.Fatalf("operating system = %q (%s), want pass", got.Status, got.Detail)
	}
	if got := find(t, checks, CheckAnsible); got.Status != StatusPass {
		t.Fatalf("ansible = %q, want pass", got.Status)
	}
}

func TestAnUnsupportedOperatingSystemFailsAndSaysWhichItIs(t *testing.T) {
	client := fakeClient{out: map[string]string{
		osReleaseCommand: "ID=alpine\n",
		ansibleCommand:   "/usr/bin/ansible-playbook\n",
	}}
	dial := func(context.Context, config.Config) (remote.Client, string, error) {
		return client, "203.0.113.10", nil
	}

	checks := Run(context.Background(), configDir(t, "hosts:\n  - 203.0.113.10\n"), dial)

	got := find(t, checks, CheckOperatingSystem)
	if got.Status != StatusFail {
		t.Fatalf("operating system = %q, want fail", got.Status)
	}
	if !strings.Contains(got.Detail, "alpine") {
		t.Fatalf("the detail does not name the distribution: %q", got.Detail)
	}
}

func TestAMissingAnsibleFailsOnItsOwn(t *testing.T) {
	client := fakeClient{
		out: map[string]string{osReleaseCommand: "ID=ubuntu\n"},
		err: map[string]error{ansibleCommand: errors.New("exit status 1")},
	}
	dial := func(context.Context, config.Config) (remote.Client, string, error) {
		return client, "203.0.113.10", nil
	}

	checks := Run(context.Background(), configDir(t, "hosts:\n  - 203.0.113.10\n"), dial)

	if got := find(t, checks, CheckOperatingSystem); got.Status != StatusPass {
		t.Fatalf("a missing ansible should not fail the OS check: %q", got.Status)
	}
	if got := find(t, checks, CheckAnsible); got.Status != StatusFail {
		t.Fatalf("ansible = %q, want fail", got.Status)
	}
}

func TestOKReportsWhetherAnythingFailed(t *testing.T) {
	if !OK([]Check{{Status: StatusPass}, {Status: StatusSkip}}) {
		t.Fatal("pass and skip should count as OK")
	}
	if OK([]Check{{Status: StatusPass}, {Status: StatusFail}}) {
		t.Fatal("a failure should not count as OK")
	}
}

// TestDoctorAgainstTheTestMachine runs the real checks against the throwaway
// VPS. It is the only test here that opens a connection.
func TestDoctorAgainstTheTestMachine(t *testing.T) {
	host := os.Getenv("DEVMACHINE_TEST_HOST")
	port := os.Getenv("DEVMACHINE_TEST_PORT")
	key := os.Getenv("DEVMACHINE_TEST_KEY")
	if host == "" || port == "" || key == "" {
		t.Skip("no test VPS: run `eval \"$(scripts/fake-vps.sh env)\"` first")
	}

	dir := t.TempDir()
	body := "hosts:\n  - " + host + "\nport: " + port + "\nuser: root\nkey: " + key + "\n"
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatalf("DEVMACHINE_TEST_PORT is not a number: %v", err)
	}

	checks := Run(context.Background(), dir, remote.Dial)

	if got := find(t, checks, CheckConnection); got.Status != StatusPass {
		t.Fatalf("connection = %q (%s), want pass", got.Status, got.Detail)
	}
	if got := find(t, checks, CheckOperatingSystem); got.Status != StatusPass {
		t.Fatalf("operating system = %q (%s), want pass", got.Status, got.Detail)
	}
	// Ansible is not installed on a fresh machine. Reporting that honestly is
	// the point of the check, so this asserts the failure rather than hiding it.
	if got := find(t, checks, CheckAnsible); got.Status != StatusFail {
		t.Fatalf("ansible = %q, want fail on a machine that has none", got.Status)
	}
}
