// Package dns answers whether a name is really serving, from outside.
//
// Checking from outside is the point: a machine can be perfectly healthy while
// the name nobody can resolve, or the certificate nobody trusts, is what is
// broken.
package dns

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// The status of one step.
const (
	StatusPass = "pass"
	StatusFail = "fail"
	// StatusSkip means the step could not run because an earlier one failed.
	StatusSkip = "skip"
)

// The steps, in the order they run.
const (
	StepDNS  = "dns"
	StepTLS  = "tls"
	StepHTTP = "http"
)

const checkTimeout = 10 * time.Second

// Seams, so the tests can point the checks at a local server.
var (
	realLookup     = lookupHost
	lookup         = realLookup
	realHTTPSPort  = "443"
	httpsPort      = realHTTPSPort
	realHTTPClient = &http.Client{Timeout: checkTimeout}
	httpClient     = realHTTPClient
)

// Step is one question and its answer.
type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Report is what a name looks like from outside.
type Report struct {
	Host  string `json:"host"`
	Steps []Step `json:"steps"`
}

// OK reports whether every step passed or was skipped.
func (r Report) OK() bool {
	for _, s := range r.Steps {
		if s.Status == StatusFail {
			return false
		}
	}
	return true
}

// Status resolves the name, checks its certificate and makes one request.
//
// A step that cannot run because an earlier one failed is skipped, not failed:
// a cascade of failures hides the one that matters.
func Status(ctx context.Context, host string) Report {
	r := Report{Host: host}

	addresses, err := lookup(ctx, host)
	if err != nil {
		r.Steps = append(r.Steps,
			Step{Name: StepDNS, Status: StatusFail, Detail: err.Error()},
			Step{Name: StepTLS, Status: StatusSkip, Detail: "the name does not resolve"},
			Step{Name: StepHTTP, Status: StatusSkip, Detail: "the name does not resolve"},
		)
		return r
	}
	r.Steps = append(r.Steps, Step{
		Name: StepDNS, Status: StatusPass, Detail: strings.Join(addresses, ", "),
	})

	target := "https://" + net.JoinHostPort(host, httpsPort) + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		r.Steps = append(r.Steps,
			Step{Name: StepTLS, Status: StatusFail, Detail: err.Error()},
			Step{Name: StepHTTP, Status: StatusSkip, Detail: "the request could not be built"},
		)
		return r
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		// Reaching the server and failing the handshake is a TLS problem;
		// anything else is still reported here, because from outside they look
		// the same until the certificate is seen.
		r.Steps = append(r.Steps,
			Step{Name: StepTLS, Status: StatusFail, Detail: err.Error()},
			Step{Name: StepHTTP, Status: StatusSkip, Detail: "the connection failed"},
		)
		return r
	}
	defer resp.Body.Close()

	detail := "the certificate was accepted"
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		detail = fmt.Sprintf("valid until %s", cert.NotAfter.UTC().Format(time.DateOnly))
	}
	r.Steps = append(r.Steps, Step{Name: StepTLS, Status: StatusPass, Detail: detail})

	// A 4xx means the server answered. Whether that path exists is a different
	// question from whether the name is serving.
	status := StatusPass
	if resp.StatusCode >= 500 {
		status = StatusFail
	}
	r.Steps = append(r.Steps, Step{
		Name: StepHTTP, Status: status, Detail: fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
	})
	return r
}

func lookupHost(ctx context.Context, host string) ([]string, error) {
	var resolver net.Resolver
	return resolver.LookupHost(ctx, host)
}
