package remote

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
)

func TestResolveKeepsALiteralAddress(t *testing.T) {
	m := config.Machine{Name: "main", Hosts: []config.Host{{Address: "203.0.113.10"}}}

	got, err := Resolve(m)
	if err != nil {
		t.Fatalf("Resolve returned %v", err)
	}
	if len(got) != 1 || got[0] != "203.0.113.10" {
		t.Fatalf("got %#v", got)
	}
}

func TestResolveKeepsTheConfiguredOrder(t *testing.T) {
	m := config.Machine{Name: "main", Hosts: []config.Host{
		{Address: "100.64.0.5"},
		{Address: "203.0.113.10"},
	}}

	got, _ := Resolve(m)
	if len(got) != 2 || got[0] != "100.64.0.5" || got[1] != "203.0.113.10" {
		t.Fatalf("the order was not preserved: %#v", got)
	}
}

func TestResolveDropsTailscaleWhenTheBinaryIsMissing(t *testing.T) {
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPath = realLookPath })

	m := config.Machine{Name: "main", Hosts: []config.Host{
		{Address: "tailscale:vps"},
		{Address: "203.0.113.10"},
	}}

	got, err := Resolve(m)
	if err != nil {
		t.Fatalf("Resolve returned %v", err)
	}
	if len(got) != 1 || got[0] != "203.0.113.10" {
		t.Fatalf("the tailscale entry was not dropped: %#v", got)
	}
}

func TestResolveExpandsTailscaleWhenItIsAvailable(t *testing.T) {
	lookPath = func(string) (string, error) { return "/usr/bin/tailscale", nil }
	tailscaleIP = func(string) (string, error) { return "100.64.0.5", nil }
	t.Cleanup(func() { lookPath = realLookPath; tailscaleIP = realTailscaleIP })

	m := config.Machine{Name: "main", Hosts: []config.Host{{Address: "tailscale:vps"}}}

	got, _ := Resolve(m)
	if len(got) != 1 || got[0] != "100.64.0.5" {
		t.Fatalf("got %#v", got)
	}
}

func TestResolveDropsTailscaleWhenThePeerIsUnknown(t *testing.T) {
	lookPath = func(string) (string, error) { return "/usr/bin/tailscale", nil }
	tailscaleIP = func(string) (string, error) { return "", errors.New("no such peer") }
	t.Cleanup(func() { lookPath = realLookPath; tailscaleIP = realTailscaleIP })

	m := config.Machine{Name: "main", Hosts: []config.Host{
		{Address: "tailscale:vps"},
		{Address: "203.0.113.10"},
	}}

	got, _ := Resolve(m)
	if len(got) != 1 || got[0] != "203.0.113.10" {
		t.Fatalf("got %#v", got)
	}
}

func TestResolveFailsWhenEveryAddressWasDropped(t *testing.T) {
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPath = realLookPath })

	m := config.Machine{Name: "main", Hosts: []config.Host{{Address: "tailscale:vps"}}}

	_, err := Resolve(m)
	if err == nil {
		t.Fatal("expected an error when nothing is left to try")
	}
	if !strings.Contains(err.Error(), "tailscale") {
		t.Fatalf("the error does not say what was dropped or why: %v", err)
	}
}

// testMachine describes the throwaway VPS from scripts/fake-vps.sh. Without it
// the integration tests skip, so `go test ./...` still works where there is no
// VM.
func testMachine(t *testing.T) config.Machine {
	t.Helper()

	host := os.Getenv("DEVMACHINE_TEST_HOST")
	port := os.Getenv("DEVMACHINE_TEST_PORT")
	key := os.Getenv("DEVMACHINE_TEST_KEY")
	if host == "" || port == "" || key == "" {
		t.Skip("no test VPS: run `eval \"$(scripts/fake-vps.sh env)\"` first")
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("DEVMACHINE_TEST_PORT is not a number: %v", err)
	}

	user := os.Getenv("DEVMACHINE_TEST_USER")
	if user == "" {
		user = "root"
	}
	return config.Machine{
		Name:  "sandbox",
		Hosts: []config.Host{{Address: host}},
		User:  user,
		Port:  n,
		Key:   key,
	}
}

func TestDialReachesTheTestMachine(t *testing.T) {
	m := testMachine(t)

	client, address, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Dial returned %v", err)
	}
	defer client.Close()

	if address != m.Hosts[0].Address {
		t.Fatalf("connected through %q, want %q", address, m.Hosts[0].Address)
	}
}

func TestRunReturnsTheCommandOutput(t *testing.T) {
	m := testMachine(t)

	client, _, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Dial returned %v", err)
	}
	defer client.Close()

	out, err := client.Run(context.Background(), "id -un")
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if strings.TrimSpace(out) != m.User {
		t.Fatalf("got %q, want %q", strings.TrimSpace(out), m.User)
	}
}

func TestRunReportsACommandThatFailed(t *testing.T) {
	m := testMachine(t)

	client, _, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Dial returned %v", err)
	}
	defer client.Close()

	if _, err := client.Run(context.Background(), "exit 3"); err == nil {
		t.Fatal("expected an error from a command that exited non-zero")
	}
}

func TestDialFallsBackToTheNextAddress(t *testing.T) {
	m := testMachine(t)
	// An address in the documentation range never answers, so the fallback is
	// the only way this can succeed.
	m.Hosts = append([]config.Host{{Address: "203.0.113.1"}}, m.Hosts...)

	client, address, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Dial returned %v", err)
	}
	defer client.Close()

	if address == "203.0.113.1" {
		t.Fatal("Dial claimed to reach an address that cannot answer")
	}
}

func TestDialSaysEveryAddressFailed(t *testing.T) {
	m := config.Machine{
		Name:  "main",
		Hosts: []config.Host{{Address: "203.0.113.1"}, {Address: "203.0.113.2"}},
		User:  "root",
		Port:  22,
	}

	_, _, err := Dial(context.Background(), m, "")
	if err == nil {
		t.Fatal("expected an error when no address answers")
	}
	for _, want := range []string{"203.0.113.1", "203.0.113.2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not name %q: %v", want, err)
		}
	}
}

func TestDialLogsInAsTheUserItIsGiven(t *testing.T) {
	m := testMachine(t)

	// An account that does not exist cannot authenticate. Failing here is what
	// proves the override reached the server instead of being ignored in
	// favour of the machine's admin login, which would have succeeded.
	if _, _, err := Dial(context.Background(), m, "nobody-here"); err == nil {
		t.Fatal("Dial succeeded as a user that does not exist on the machine")
	}
}

func TestDialFallsBackToTheAdminUserWhenGivenNone(t *testing.T) {
	m := testMachine(t)

	client, _, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Dial returned %v", err)
	}
	defer client.Close()

	out, err := client.Run(context.Background(), "id -un")
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if strings.TrimSpace(out) != m.User {
		t.Fatalf("logged in as %q, want the machine's admin user %q", strings.TrimSpace(out), m.User)
	}
}

func TestDialTellsARefusedLoginApartFromAnUnreachableMachine(t *testing.T) {
	m := testMachine(t)

	_, _, err := Dial(context.Background(), m, "nobody-here")
	if err == nil {
		t.Fatal("Dial succeeded as a user that does not exist")
	}
	// The machine answered; only the login failed. Saying "no address
	// answered" would send someone to check the network for nothing.
	if strings.Contains(err.Error(), "no address answered") {
		t.Fatalf("a refused login was reported as an unreachable machine: %v", err)
	}
	for _, want := range []string{"refused the login", "nobody-here"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not say %q: %v", want, err)
		}
	}
}
