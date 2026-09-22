package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/dns"
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

// configWithDomain is a one-machine configuration with a `domain` set, for a
// command that falls back to it when no zone or host was given.
func configWithDomain(t *testing.T, domain string) string {
	t.Helper()
	return configWith(t, fmt.Sprintf("machines:\n  - name: main\n    hosts: [203.0.113.10]\ndomain: %s\n", domain))
}

// fake is a dns.Provider a test drives by hand, recording what was written to
// it so a test can assert on the exact call.
type fake struct {
	records    []dns.Record
	listErr    error
	upserts    int
	lastRecord dns.Record
	lastZone   string
}

func (f *fake) Name() string { return "fake" }

func (f *fake) List(context.Context, string) ([]dns.Record, error) { return f.records, f.listErr }

func (f *fake) Upsert(_ context.Context, zone string, r dns.Record) error {
	f.upserts++
	f.lastRecord, f.lastZone = r, zone
	return nil
}

func (f *fake) Delete(_ context.Context, zone string, r dns.Record) error {
	f.lastRecord, f.lastZone = r, zone
	return nil
}

// stubChoose replaces chooseDNS with one that always resolves to provider p,
// holding zone, without dialling anything.
func stubChoose(provider, zone string, p dns.Provider) func() {
	orig := chooseDNS
	chooseDNS = func(context.Context, *options, string, string, string, io.Writer) (dns.Choice, target, error) {
		return dns.Choice{Provider: p, Name: provider, Zone: zone, Why: fmt.Sprintf("%s holds %s", provider, zone)},
			target{machine: config.Machine{Name: "main"}}, nil
	}
	return func() { chooseDNS = orig }
}

func TestDNSListPrintsARecordPerLine(t *testing.T) {
	defer stubChoose("hostinger", "example.com", &fake{records: []dns.Record{
		{Name: "www", Type: "A", Value: "198.51.100.10", TTL: 3600},
	}})()

	out, err := execute(t, "--config", configDir(t), "dns", "list", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "www") || !strings.Contains(out, "198.51.100.10") {
		t.Fatalf("got %q", out)
	}
}

func TestDNSListJSONSaysWhichZoneAndProvider(t *testing.T) {
	defer stubChoose("cloudflare", "client.example.net", &fake{
		records: []dns.Record{{Name: "www", Type: "A", Value: "198.51.100.10"}},
	})()

	out, _, err := executeSplit(t, "--config", configDir(t), "dns", "list", "client.example.net", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Zone     string       `json:"zone"`
		Provider string       `json:"provider"`
		Records  []dns.Record `json:"records"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.Zone != "client.example.net" || got.Provider != "cloudflare" {
		t.Fatalf("got %#v", got)
	}
}

func TestDNSListFallsBackToTheConfiguredDomain(t *testing.T) {
	defer stubChoose("hostinger", "example.com", &fake{})()

	if _, err := execute(t, "--config", configWithDomain(t, "example.com"), "dns", "list"); err != nil {
		t.Fatal(err)
	}
}

func TestDNSListSaysWhichProviderItUsedOnStderr(t *testing.T) {
	defer stubChoose("hostinger", "example.com", &fake{})()

	_, errOut, err := executeSplit(t, "--config", configDir(t), "dns", "list", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	// stdout is data; which provider answered and why is diagnostics.
	if !strings.Contains(errOut, "hostinger") {
		t.Fatalf("stderr does not say what answered: %q", errOut)
	}
}

func TestDNSCheckFindsARecordThatIsThere(t *testing.T) {
	defer stubChoose("cloudflare", "client.example.net", &fake{records: []dns.Record{
		{Name: "app", Type: "A", Value: "198.51.100.10", TTL: 3600},
	}})()

	out, _, err := executeSplit(t, "--config", configDir(t), "dns", "check", "app.client.example.net", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Name     string       `json:"name"`
		Zone     string       `json:"zone"`
		Provider string       `json:"provider"`
		Found    bool         `json:"found"`
		Records  []dns.Record `json:"records"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if !got.Found || got.Records[0].Value != "198.51.100.10" {
		t.Fatalf("got %#v", got)
	}
}

func TestDNSCheckExitsNonZeroWhenItIsNotPointed(t *testing.T) {
	defer stubChoose("cloudflare", "client.example.net", &fake{})()

	// A check is asked by a script and by a skill. "Not there" has to be
	// readable from the exit code, not only from the words.
	if _, err := execute(t, "--config", configDir(t), "dns", "check", "app.client.example.net"); err == nil {
		t.Fatal("a name that is not pointed exited 0")
	}
}
