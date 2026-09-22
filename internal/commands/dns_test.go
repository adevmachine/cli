package commands

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/remote"
)

// configDir is a plain, one-machine configuration: the ordinary starting
// point for a DNS command test.
func configDir(t *testing.T) string {
	t.Helper()
	return configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")
}

// fakeZoner is a dnsZoner a test builds by hand, for `dns providers`.
type fakeZoner struct {
	name  string
	zones []string
	err   error
}

func (f fakeZoner) Name() string { return f.name }

func (f fakeZoner) Zones(context.Context) ([]string, error) { return f.zones, f.err }

// stubInstalled replaces dnsInstalled with one that answers with these
// providers, and dial with one that never really connects.
func stubInstalled(providers []fakeZoner) func() {
	origInstalled, origDial := dnsInstalled, dial
	dnsInstalled = func(string, string, remote.Client) ([]dnsZoner, error) {
		out := make([]dnsZoner, len(providers))
		for i, p := range providers {
			out[i] = p
		}
		return out, nil
	}
	dial = func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return fakeRemote{}, "203.0.113.10", nil
	}
	return func() { dnsInstalled = origInstalled; dial = origDial }
}

func TestDNSProvidersListsEachOneAndItsZones(t *testing.T) {
	defer stubInstalled([]fakeZoner{
		{name: "hostinger", zones: []string{"example.com"}},
		{name: "cloudflare", zones: []string{"client.example.net"}},
	})()

	out, err := execute(t, "--config", configDir(t), "--format", "json", "dns", "providers")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Providers []struct {
			Name  string   `json:"name"`
			Zones []string `json:"zones"`
			Error string   `json:"error,omitempty"`
		} `json:"providers"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got.Providers) != 2 || got.Providers[1].Zones[0] != "client.example.net" {
		t.Fatalf("got %#v", got.Providers)
	}
}

func TestDNSProvidersReportsOneThatCannotBeAsked(t *testing.T) {
	defer stubInstalled([]fakeZoner{
		{name: "hostinger", err: errors.New("the token was rejected")},
	})()

	out, err := execute(t, "--config", configDir(t), "dns", "providers")
	// A broken provider is what this command exists to show, so listing is
	// still a success.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "the token was rejected") {
		t.Fatalf("got %q", out)
	}
}

func TestDNSProvidersSaysWhenNoneAreInstalled(t *testing.T) {
	defer stubInstalled(nil)()

	out, err := execute(t, "--config", configDir(t), "dns", "providers")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "packages list") {
		t.Fatalf("it does not say where to find one: %q", out)
	}
}
