package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/credentials"
	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
)

type fakeClient struct {
	out map[string]string
	err map[string]error
	// input is the answer to a command fed from standard input, which is how
	// the credentials are looked for.
	input string
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

func (f fakeClient) RunInput(ctx context.Context, command string, _ io.Reader) (string, error) {
	if f.input != "" {
		return f.input, nil
	}
	return f.Run(ctx, command)
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

	checks := Run(context.Background(), configDir(t, ""), "", dial, nil)

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

	checks := Run(context.Background(), configDir(t, "domain: example.com\n"), "", dial, nil)

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

	checks := Run(context.Background(), configDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"), "", dial, nil)

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

	checks := Run(context.Background(), configDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"), "", dial, nil)

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

	checks := Run(context.Background(), configDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"), "", dial, nil)

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

	checks := Run(context.Background(), configDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"), "", dial, nil)

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

	checks := Run(context.Background(), dir, "", remote.Dial, nil)

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
	checks := Run(context.Background(), configDir(t, body), "", dial, nil)

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

	checks := Run(context.Background(), configDir(t, body), "", dial, nil)

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

	Run(context.Background(), configDir(t, body), "sandbox", dial, nil)

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

	checks := Run(context.Background(), configDir(t, body), "", dial, nil)

	if got := find(t, checks, CheckConfiguration); !strings.Contains(got.Detail, "2 workspace") {
		t.Fatalf("the detail does not count the workspaces: %q", got.Detail)
	}
}

// machineWith is the usual one-machine configuration these checks run against.
const machineWith = "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"

// working answers every check up to the credentials, so a test can look at
// what comes after them.
func working(present string) fakeClient {
	return fakeClient{
		out: map[string]string{
			osReleaseCommand: "ID=ubuntu\n",
			ansibleCommand:   "/usr/bin/ansible-playbook\n",
		},
		input: present,
	}
}

func dialling(client remote.Client) Dialer {
	return func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return client, "203.0.113.10", nil
	}
}

func login(name, workspace, user string) credentials.Declared {
	return credentials.Declared{
		Credential: packages.Credential{
			Name: name, Kind: packages.KindManual,
			Command: name + " auth login", StoredAt: "~/.config/" + name + "/hosts.yml",
		},
		Package: name + "-login", Workspace: workspace, LinuxUser: user,
	}
}

func TestACredentialThatIsThereIsItsOwnPassingCheck(t *testing.T) {
	wanted := []credentials.Declared{login("gh", "", "")}
	checks := Run(context.Background(), configDir(t, machineWith), "", dialling(working("gh\tyes\n")), wanted)

	if got := find(t, checks, "credential: gh"); got.Status != StatusPass {
		t.Fatalf("credential: gh = %q (%s), want pass", got.Status, got.Detail)
	}
}

func TestAMissingCredentialFailsAndSaysHowToFixIt(t *testing.T) {
	wanted := []credentials.Declared{login("claude", "alice", "alice")}
	checks := Run(context.Background(), configDir(t, machineWith), "", dialling(working("alice/claude\tno\n")), wanted)

	got := find(t, checks, "credential: alice/claude")
	if got.Status != StatusFail {
		t.Fatalf("credential = %q, want fail", got.Status)
	}
	if !strings.Contains(got.Detail, "devmachine login claude --workspace alice") {
		t.Fatalf("the detail does not say how to fix it: %q", got.Detail)
	}
}

func TestAMissingSecretSaysToStoreItAndPushIt(t *testing.T) {
	wanted := []credentials.Declared{{
		Credential: packages.Credential{Name: "token", Kind: packages.KindSecret, Env: "TOKEN"},
		Package:    "provider",
	}}
	checks := Run(context.Background(), configDir(t, machineWith), "", dialling(working("token\tno\n")), wanted)

	got := find(t, checks, "credential: token")
	for _, want := range []string{"devmachine secrets set token", "credentials push"} {
		if !strings.Contains(got.Detail, want) {
			t.Fatalf("the detail leaves out %q: %q", want, got.Detail)
		}
	}
}

func TestACredentialWithNowhereToLookIsNotReportedMissing(t *testing.T) {
	// "I cannot tell" and "it is not there" are different things, and only one
	// of them is a failure.
	wanted := []credentials.Declared{{
		Credential: packages.Credential{Name: "gh", Kind: packages.KindManual, Command: "gh auth login"},
		Package:    "gh-login",
	}}
	checks := Run(context.Background(), configDir(t, machineWith), "", dialling(working("")), wanted)

	if got := find(t, checks, "credential: gh"); got.Status != StatusSkip {
		t.Fatalf("credential: gh = %q (%s), want skip", got.Status, got.Detail)
	}
	if !OK(checks) {
		t.Fatal("a credential nobody can look for should not fail the machine")
	}
}

func TestAMachineWithNothingDeclaredReportsNoCredential(t *testing.T) {
	checks := Run(context.Background(), configDir(t, machineWith), "", dialling(working("")), nil)

	for _, c := range checks {
		if strings.HasPrefix(c.Name, credentialPrefix) {
			t.Fatalf("a machine with no packages reported %q", c.Name)
		}
	}
	if !OK(checks) {
		t.Fatalf("a machine with nothing declared should pass: %#v", checks)
	}
}

func TestAnUnreachableMachineSkipsTheCredentialChecks(t *testing.T) {
	dial := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return nil, "", errors.New("connection refused")
	}
	wanted := []credentials.Declared{login("gh", "", "")}

	checks := Run(context.Background(), configDir(t, machineWith), "", dial, wanted)

	// The cascade covers it because it is in `order`, not because anybody
	// remembered to add it to a list of things to skip.
	if got := find(t, checks, "credential: gh"); got.Status != StatusSkip {
		t.Fatalf("credential: gh = %q, want skip", got.Status)
	}
}

func writeDNSProviderPackage(t *testing.T, dir, name string) {
	t.Helper()
	pkgDir := filepath.Join(packages.LocalDir(dir), name)
	if err := os.MkdirAll(filepath.Join(pkgDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(
		"format: 1\nname: %s\nscope: machine\nsummary: A package written by a test.\n"+
			"kind: dns\nentrypoint: bin/provider\ncommands: [\"*\"]\n"+
			"credentials:\n  - name: %s\n    kind: secret\n    env: %s_TOKEN\n",
		name, name, strings.ToUpper(name))
	if err := os.WriteFile(packages.ManifestPath(pkgDir), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func lockDNSProviders(t *testing.T, dir, machine string, names ...string) {
	t.Helper()
	entries := make([]packages.LockEntry, len(names))
	for i, n := range names {
		entries[i] = packages.LockEntry{Name: n, Source: packages.SourceLocal}
	}
	lock := packages.Lock{
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
		Machines:  map[string][]packages.LockEntry{machine: entries},
	}
	if err := packages.SaveLock(dir, lock); err != nil {
		t.Fatal(err)
	}
}

func dirWithTwoProviders(t *testing.T) string {
	t.Helper()
	dir := configDir(t, machineWith)
	writeDNSProviderPackage(t, dir, "hostinger")
	writeDNSProviderPackage(t, dir, "cloudflare")
	lockDNSProviders(t, dir, "main", "hostinger", "cloudflare")
	return dir
}

func dirWithBrokenProvider(t *testing.T) string {
	t.Helper()
	dir := configDir(t, machineWith)
	writeDNSProviderPackage(t, dir, "hostinger")
	lockDNSProviders(t, dir, "main", "hostinger")
	return dir
}

func dirWithNoProviders(t *testing.T) string {
	t.Helper()
	return configDir(t, machineWith)
}

// dnsAwareClient answers the operating system and ansible checks like any
// other working machine, and also answers a DNS package's `zones` call: with
// the zones it holds, or with the error kind it should fail with.
type dnsAwareClient struct {
	fakeClient
	zones map[string][]string
	fails map[string]string
}

func (d dnsAwareClient) Run(ctx context.Context, command string) (string, error) {
	if strings.Contains(command, " zones") {
		for name, zones := range d.zones {
			if strings.Contains(command, "/"+name+"/") {
				body, err := json.Marshal(struct {
					Zones []string `json:"zones"`
				}{zones})
				return string(body), err
			}
		}
		for name, kind := range d.fails {
			if strings.Contains(command, "/"+name+"/") {
				return fmt.Sprintf(`{"error":{"kind":%q,"message":"the token was rejected"}}`, kind),
					errors.New("exit status 1")
			}
		}
	}
	return d.fakeClient.Run(ctx, command)
}

func TestDoctorReportsOneCheckPerInstalledProvider(t *testing.T) {
	client := dnsAwareClient{fakeClient: working(""), zones: map[string][]string{
		"hostinger": {"example.com"}, "cloudflare": {"client.example.net"},
	}}
	checks := Run(context.Background(), dirWithTwoProviders(t), "", dialling(client), nil)

	var seen []string
	for _, c := range checks {
		if strings.HasPrefix(c.Name, CheckDNS) {
			seen = append(seen, c.Name)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("got %v", seen)
	}
}

func TestDoctorNamesTheZonesAProviderHolds(t *testing.T) {
	client := dnsAwareClient{fakeClient: working(""), zones: map[string][]string{
		"hostinger": {"example.com"}, "cloudflare": {"client.example.net"},
	}}
	checks := Run(context.Background(), dirWithTwoProviders(t), "", dialling(client), nil)
	c := find(t, checks, "dns: cloudflare")
	if c.Status != StatusPass {
		t.Fatalf("got %#v", c)
	}
	if !strings.Contains(c.Detail, "client.example.net") {
		t.Fatalf("the detail does not say what it holds: %#v", c)
	}
}

func TestDoctorFailsAProviderWhoseTokenIsGone(t *testing.T) {
	client := dnsAwareClient{fakeClient: working(""), fails: map[string]string{"hostinger": "unauthenticated"}}
	checks := Run(context.Background(), dirWithBrokenProvider(t), "", dialling(client), nil)
	c := find(t, checks, "dns: hostinger")
	if c.Status != StatusFail {
		t.Fatalf("got %#v", c)
	}
	if !strings.Contains(c.Detail, "secrets set") {
		t.Fatalf("the detail does not say how to fix it: %#v", c)
	}
}

func TestDoctorSaysNothingAboutDNSWhenNoProviderIsInstalled(t *testing.T) {
	checks := Run(context.Background(), dirWithNoProviders(t), "", dialling(working("")), nil)
	for _, c := range checks {
		if strings.HasPrefix(c.Name, CheckDNS) {
			// Not installing a DNS provider is a choice, not a fault.
			t.Fatalf("it complained about a provider nobody wanted: %#v", c)
		}
	}
}
