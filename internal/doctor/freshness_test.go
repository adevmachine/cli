package doctor

import (
	"errors"
	"strings"
	"testing"
)

func TestCLICheck(t *testing.T) {
	offline := errors.New("dial tcp: no route to host")
	cases := []struct {
		name            string
		current, latest string
		err             error
		status, detail  string
	}{
		{"latest", "0.7.18", "v0.7.18", nil, StatusPass, "0.7.18"},
		{"ahead", "0.8.0", "v0.7.18", nil, StatusPass, "0.8.0"},
		{"older", "0.7.17", "v0.7.18", nil, StatusWarn, "devmachine update"},
		{"offline", "0.7.17", "", offline, StatusSkip, "no route to host"},
		{"development build", "dev", "v0.7.18", nil, StatusSkip, "development build"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CLICheck(c.current, c.latest, c.err)
			if got.Name != CheckCLI || got.Status != c.status || !strings.Contains(got.Detail, c.detail) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestPackagesPinCheck(t *testing.T) {
	cases := []struct {
		name           string
		pinned, latest string
		err            error
		status, detail string
	}{
		{"latest", "v17", "v17", nil, StatusPass, "v17"},
		{"older", "v16", "v17", nil, StatusWarn, "v16, and v17 is out: run `devmachine update`"},
		{"offline", "v16", "", errors.New("timeout"), StatusSkip, "timeout"},
		{"nothing pinned", "", "v17", nil, StatusSkip, "devmachine packages pin"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PackagesPinCheck(c.pinned, c.latest, c.err)
			if got.Name != CheckPackagesPin || got.Status != c.status || !strings.Contains(got.Detail, c.detail) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestAWarningIsNotAFailure(t *testing.T) {
	if !OK([]Check{CLICheck("0.7.1", "v0.7.18", nil), PackagesPinCheck("v1", "v17", nil)}) {
		t.Fatal("an old version must never fail doctor")
	}
}
