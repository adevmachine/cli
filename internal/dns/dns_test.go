package dns

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func find(t *testing.T, r Report, step string) Step {
	t.Helper()
	for _, s := range r.Steps {
		if s.Name == step {
			return s
		}
	}
	t.Fatalf("no step named %q in %#v", step, r.Steps)
	return Step{}
}

func TestANameThatDoesNotResolveFailsAndSkipsTheRest(t *testing.T) {
	lookup = func(context.Context, string) ([]string, error) {
		return nil, errors.New("no such host")
	}
	t.Cleanup(func() { lookup = realLookup })

	r := Status(context.Background(), "absent.example.com")

	if got := find(t, r, StepDNS); got.Status != StatusFail {
		t.Fatalf("dns = %q, want fail", got.Status)
	}
	// Reporting TLS and HTTP as failures too would bury the one thing that is
	// actually wrong.
	for _, step := range []string{StepTLS, StepHTTP} {
		if got := find(t, r, step); got.Status != StatusSkip {
			t.Fatalf("%s = %q, want skip", step, got.Status)
		}
	}
	if r.OK() {
		t.Fatal("a report with a failure should not be OK")
	}
}

func TestAResolvedNameReportsItsAddresses(t *testing.T) {
	lookup = func(context.Context, string) ([]string, error) {
		return []string{"203.0.113.10"}, nil
	}
	t.Cleanup(func() { lookup = realLookup })

	r := Status(context.Background(), "example.com")

	got := find(t, r, StepDNS)
	if got.Status != StatusPass {
		t.Fatalf("dns = %q (%s), want pass", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, "203.0.113.10") {
		t.Fatalf("the detail does not name the address: %q", got.Detail)
	}
}

// serverFor points the checks at a local TLS server, so a real domain and a
// real certificate authority are never needed.
func serverFor(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)

	host, port, err := net.SplitHostPort(mustURL(t, srv.URL).Host)
	if err != nil {
		t.Fatal(err)
	}

	lookup = func(context.Context, string) ([]string, error) { return []string{host}, nil }
	httpsPort = port
	httpClient = srv.Client()
	t.Cleanup(func() {
		lookup, httpsPort, httpClient = realLookup, realHTTPSPort, realHTTPClient
	})
	return srv
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestAServingNameReportsItsStatusCode(t *testing.T) {
	serverFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	r := Status(context.Background(), "127.0.0.1")

	if got := find(t, r, StepTLS); got.Status != StatusPass {
		t.Fatalf("tls = %q (%s), want pass", got.Status, got.Detail)
	}
	got := find(t, r, StepHTTP)
	if got.Status != StatusPass {
		t.Fatalf("http = %q (%s), want pass", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, "204") {
		t.Fatalf("the detail does not carry the status code: %q", got.Detail)
	}
}

func TestAServerErrorIsReportedAsAFailure(t *testing.T) {
	serverFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	r := Status(context.Background(), "127.0.0.1")

	got := find(t, r, StepHTTP)
	if got.Status != StatusFail {
		t.Fatalf("http = %q, want fail for a 502", got.Status)
	}
	if !strings.Contains(got.Detail, "502") {
		t.Fatalf("the detail does not carry the status code: %q", got.Detail)
	}
}

func TestAClientErrorStillCountsAsServing(t *testing.T) {
	// A 404 means the server answered. Whether that path exists is not what
	// this command is asking about.
	serverFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	r := Status(context.Background(), "127.0.0.1")

	if got := find(t, r, StepHTTP); got.Status != StatusPass {
		t.Fatalf("http = %q, want pass for a 404", got.Status)
	}
}

func TestATLSFailureSkipsTheHTTPStep(t *testing.T) {
	// A plain HTTP server on the TLS port makes the handshake fail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)

	host, port, _ := net.SplitHostPort(mustURL(t, srv.URL).Host)
	lookup = func(context.Context, string) ([]string, error) { return []string{host}, nil }
	httpsPort = port
	httpClient = &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	t.Cleanup(func() { lookup, httpsPort, httpClient = realLookup, realHTTPSPort, realHTTPClient })

	r := Status(context.Background(), "127.0.0.1")

	if got := find(t, r, StepTLS); got.Status != StatusFail {
		t.Fatalf("tls = %q, want fail", got.Status)
	}
	if got := find(t, r, StepHTTP); got.Status != StatusSkip {
		t.Fatalf("http = %q, want skip after a TLS failure", got.Status)
	}
}

func TestOKIsTrueWhenEveryStepPassed(t *testing.T) {
	serverFor(t, func(w http.ResponseWriter, r *http.Request) {})

	if !Status(context.Background(), "127.0.0.1").OK() {
		t.Fatal("a report with no failure should be OK")
	}
}
