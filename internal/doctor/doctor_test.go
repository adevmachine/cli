package doctor

import (
	"context"
	"errors"
	"io"
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

func (f fakeClient) Stream(ctx context.Context, command string, stdout, _ io.Writer) error {
	out, err := f.Run(ctx, command)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, out)
	return err
}

func (f fakeClient) Upload(context.Context, string, io.Reader) error { return nil }

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
	dial := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		dialled = true
		return nil, "", nil
	}

	checks := Run(context.Background(), configDir(t, ""), "", dial)

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
	dial := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		dialled = true
		return nil, "", nil
	}

	checks := Run(context.Background(), configDir(t, "domain: example.com\n"), "", dial)

	if got := find(t, checks, CheckConfiguration); got.Status != StatusFail {
		t.Fatalf("configuration = %q, want fail", got.Status)
	}
	if dialled {
		t.Fatal("it tried to connect with an invalid configuration")
	}
}

func TestAnUnreachableMachineSkipsTheRemoteChecks(t *testing.T) {
	dial := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return nil, "", errors.New("connection refused")
	}

	checks := Run(context.Background(), configDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"), "", dial)

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
	dial := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return client, "203.0.113.10", nil
	}

	checks := Run(context.Background(), configDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"), "", dial)

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
	dial := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return client, "203.0.113.10", nil
	}

	checks := Run(context.Background(), configDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"), "", dial)

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
	dial := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return client, "203.0.113.10", nil
	}

	checks := Run(context.Background(), configDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"), "", dial)

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
	body := "machines:\n  - name: sandbox\n    hosts: [" + host + "]\n    port: " + port + "\n    user: root\n    key: " + key + "\n"
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatalf("DEVMACHINE_TEST_PORT is not a number: %v", err)
	}

	checks := Run(context.Background(), dir, "", remote.Dial)

	if got := find(t, checks, CheckConnection); got.Status != StatusPass {
		t.Fatalf("connection = %q (%s), want pass", got.Status, got.Detail)
	}
	if got := find(t, checks, CheckOperatingSystem); got.Status != StatusPass {
		t.Fatalf("operating system = %q (%s), want pass", got.Status, got.Detail)
	}
	// Whether Ansible is there depends on how far the machine has been taken:
	// a freshly created one has none, one that `sync` has run against does.
	// What this asserts is that the check answered at all — a skip here would
	// mean the cascade stopped earlier than the two passes above say it did.
	if got := find(t, checks, CheckAnsible); got.Status == StatusSkip {
		t.Fatalf("ansible = skip (%s), but the connection and the OS both passed", got.Detail)
	}
}

func TestRunReportsEachCheckOnce(t *testing.T) {
	dial := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return nil, "", errors.New("no address answered")
	}
	body := "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"
	checks := Run(context.Background(), configDir(t, body), "", dial)

	seen := map[string]int{}
	for _, c := range checks {
		seen[c.Name]++
	}
	for name, n := range seen {
		if n != 1 {
			t.Errorf("%q appears %d times; a check that failed must not also be skipped", name, n)
		}
	}
}

func TestRunRefusesToGuessBetweenSeveralMachines(t *testing.T) {
	dialled := false
	dial := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		dialled = true
		return nil, "", nil
	}
	body := "machines:\n" +
		"  - name: main\n    hosts: [203.0.113.10]\n" +
		"  - name: sandbox\n    hosts: [203.0.113.11]\n"

	checks := Run(context.Background(), configDir(t, body), "", dial)

	got := find(t, checks, CheckConfiguration)
	if got.Status != StatusFail {
		t.Fatalf("configuration = %q, want fail", got.Status)
	}
	if !strings.Contains(got.Detail, "--machine") {
		t.Fatalf("the detail does not say how to choose: %q", got.Detail)
	}
	if dialled {
		t.Fatal("it connected to a machine nobody named")
	}
}

func TestRunChecksTheMachineItWasGiven(t *testing.T) {
	var got config.Machine
	dial := func(_ context.Context, m config.Machine, _ string) (remote.Client, string, error) {
		got = m
		return fakeClient{out: map[string]string{
			osReleaseCommand: "ID=ubuntu\n",
			ansibleCommand:   "/usr/bin/ansible-playbook\n",
		}}, m.Hosts[0].Address, nil
	}
	body := "machines:\n" +
		"  - name: main\n    hosts: [203.0.113.10]\n" +
		"  - name: sandbox\n    hosts: [203.0.113.11]\n"

	Run(context.Background(), configDir(t, body), "sandbox", dial)

	if got.Name != "sandbox" {
		t.Fatalf("it checked machine %q, want sandbox", got.Name)
	}
}

func TestTheConfigurationCheckCountsTheWorkspaces(t *testing.T) {
	dial := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return nil, "", errors.New("unreachable")
	}
	body := "machines:\n  - name: main\n    hosts: [203.0.113.10]\n" +
		"workspaces:\n  - name: alice\n  - name: bob\n"

	checks := Run(context.Background(), configDir(t, body), "", dial)

	if got := find(t, checks, CheckConfiguration); !strings.Contains(got.Detail, "2 workspace") {
		t.Fatalf("the detail does not count the workspaces: %q", got.Detail)
	}
}
