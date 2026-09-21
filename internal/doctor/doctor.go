// Package doctor answers one question: can this CLI do its job right now, and
// if not, which step is broken.
package doctor

import (
	"context"
	"fmt"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/credentials"
	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
)

// The status of one check.
const (
	StatusPass = "pass"
	StatusFail = "fail"
	// StatusSkip means the check could not run because an earlier one failed.
	// It is not a pass and not a failure: reporting it as either would lie.
	StatusSkip = "skip"
)

// The checks, in the order they run.
const (
	CheckConfiguration   = "configuration"
	CheckConnection      = "connection"
	CheckOperatingSystem = "operating system"
	CheckAnsible         = "ansible"
)

// credentialPrefix names the check for one credential, so `credential: gh` and
// `credential: alice/claude` read as what they are.
const credentialPrefix = "credential: "

// CredentialCheck is what the check for one credential is called.
func CredentialCheck(d credentials.Declared) string { return credentialPrefix + credentials.Key(d) }

// The commands the remote checks run. They are constants so a test can answer
// them without guessing at the wording.
const (
	// One reader for /etc/os-release, because a second one drifts from the
	// first. remote owns it: that is where it is acted on.
	osReleaseCommand = remote.OSReleaseCommand
	ansibleCommand   = "command -v ansible-playbook"
)

// supportedIDs are the distributions this CLI claims to support. Anything else
// is reported plainly instead of half-working and failing partway through.
var supportedIDs = map[string]bool{
	"ubuntu": true,
	"debian": true,
}

// Check is one question and its answer.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Dialer opens a connection to a machine. It is a parameter so the tests do
// not need one.
type Dialer func(context.Context, config.Machine, string) (remote.Client, string, error)

// Run checks one machine and returns the results in order. It never returns an
// error: a failed check is the result, not an exception.
//
// machine names which one to check. Empty means the only configured machine,
// and with several it is an error rather than a guess.
func Run(ctx context.Context, dir, machine string, dial Dialer, wanted []credentials.Declared) []Check {
	reported := order(wanted)

	cfg, err := loadAndValidate(dir)
	if err != nil {
		return append(
			[]Check{{Name: CheckConfiguration, Status: StatusFail, Detail: err.Error()}},
			skipRest(reported, CheckConfiguration, "the configuration is not usable")...,
		)
	}
	m, err := cfg.Machine(machine)
	if err != nil {
		return append(
			[]Check{{Name: CheckConfiguration, Status: StatusFail, Detail: err.Error()}},
			skipRest(reported, CheckConfiguration, "there is no machine to check")...,
		)
	}

	checks := []Check{{
		Name:   CheckConfiguration,
		Status: StatusPass,
		Detail: fmt.Sprintf("machine %q, %d address(es), admin %s, port %d, %d workspace(s)",
			m.Name, len(m.Hosts), m.User, m.Port, len(cfg.WorkspacesOn(m.Name))),
	}}

	client, address, err := dial(ctx, m, "")
	if err != nil {
		checks = append(checks, Check{Name: CheckConnection, Status: StatusFail, Detail: err.Error()})
		return append(checks, skipRest(reported, CheckConnection, "the machine is unreachable")...)
	}
	defer client.Close()

	checks = append(checks, Check{
		Name:   CheckConnection,
		Status: StatusPass,
		Detail: fmt.Sprintf("connected through %s", address),
	})
	checks = append(checks, remoteChecks(ctx, client)...)
	return append(checks, credentialChecks(ctx, client, wanted)...)
}

func loadAndValidate(dir string) (config.Config, error) {
	cfg, err := config.Load(dir)
	if err != nil {
		return cfg, err
	}
	return cfg, cfg.Validate()
}

// order is every check this run reports, in the order they run. skipRest walks
// it, so a check added here is skipped by every cascade without anybody
// remembering to — including one credential's, which only this run knows the
// name of.
func order(wanted []credentials.Declared) []string {
	out := []string{CheckConfiguration, CheckConnection, CheckOperatingSystem, CheckAnsible}
	for _, d := range wanted {
		out = append(out, CredentialCheck(d))
	}
	return out
}

// skipRest returns a skip for every check that comes after `done`.
//
// It takes the last check already reported rather than a fixed list: reporting
// a connection as failed and then skipping it as well says two things about
// one check, and the one a reader believes is whichever they see first.
func skipRest(order []string, done, reason string) []Check {
	var out []Check
	past := false
	for _, n := range order {
		if past {
			out = append(out, Check{Name: n, Status: StatusSkip, Detail: reason})
		}
		if n == done {
			past = true
		}
	}
	return out
}

func remoteChecks(ctx context.Context, client remote.Client) []Check {
	var out []Check

	release, err := client.Run(ctx, osReleaseCommand)
	switch {
	case err != nil:
		out = append(out, Check{Name: CheckOperatingSystem, Status: StatusFail, Detail: err.Error()})
	default:
		id := remote.OSReleaseID(release)
		if supportedIDs[id] {
			out = append(out, Check{Name: CheckOperatingSystem, Status: StatusPass, Detail: id})
		} else {
			out = append(out, Check{
				Name:   CheckOperatingSystem,
				Status: StatusFail,
				Detail: fmt.Sprintf("%q is not supported yet: this CLI supports debian and ubuntu", id),
			})
		}
	}

	path, err := client.Run(ctx, ansibleCommand)
	if err != nil {
		out = append(out, Check{
			Name:   CheckAnsible,
			Status: StatusFail,
			Detail: "ansible-playbook is not on the machine; `devmachine setup` installs it",
		})
	} else {
		out = append(out, Check{Name: CheckAnsible, Status: StatusPass, Detail: strings.TrimSpace(path)})
	}
	return out
}

// credentialChecks reports one check per declared credential.
//
// A machine whose packages declare nothing has nothing to report, and reports
// nothing: an empty list is not a failure.
func credentialChecks(ctx context.Context, client remote.Client, wanted []credentials.Declared) []Check {
	if len(wanted) == 0 {
		return nil
	}

	present, err := credentials.Present(ctx, client, wanted)
	if err != nil {
		var out []Check
		for _, d := range wanted {
			out = append(out, Check{Name: CredentialCheck(d), Status: StatusFail, Detail: err.Error()})
		}
		return out
	}

	var out []Check
	for _, d := range wanted {
		check := Check{Name: CredentialCheck(d)}
		switch {
		case credentials.Place(d) == "":
			check.Status = StatusSkip
			check.Detail = fmt.Sprintf(
				"package %q did not say where it is kept, so there is nowhere to look", d.Package)
		case present[credentials.Key(d)]:
			check.Status = StatusPass
			check.Detail = credentials.Place(d)
		default:
			check.Status = StatusFail
			check.Detail = credentialFix(d)
		}
		out = append(out, check)
	}
	return out
}

// credentialFix is the command that makes a missing credential arrive. A
// failure that does not say what to do next is half a report.
func credentialFix(d credentials.Declared) string {
	if d.Kind == packages.KindManual {
		return "missing: run `" + credentials.LoginCommand(d) + "`"
	}
	return fmt.Sprintf("missing: run `devmachine secrets set %s`, then `devmachine credentials push`", d.Name)
}

// OK reports whether every check passed or was skipped.
func OK(checks []Check) bool {
	for _, c := range checks {
		if c.Status == StatusFail {
			return false
		}
	}
	return true
}
