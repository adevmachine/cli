package credentials

import (
	"fmt"
	"path"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
)

// Resolve says what happens about one credential in one workspace: one login
// shared across the machine, or one the workspace obtains for itself.
//
// First hit wins — the workspace's answer, then the configuration's, then the
// package's own recommendation — and `prefer` is already the first two, laid
// one over the other.
//
// A package that says a copy of its session does not work beats all of them.
// Asking to share such a login is refused rather than obeyed: the copy would
// land, the tool would reject it, and nothing would say why.
func Resolve(pkg string, c packages.Credential, prefer map[string]string) (string, error) {
	asked := prefer[c.Name]

	if !c.Shareable {
		if asked == config.CredentialMachine {
			return packages.ScopeWorkspace, fmt.Errorf(
				"credential %q cannot be shared: the package %q does not say `shareable: true` about it, "+
					"so a copy of %s would not work on another account. Ask for %q instead",
				c.Name, pkg, c.StoredAt, config.CredentialOwn)
		}
		return packages.ScopeWorkspace, nil
	}

	switch asked {
	case config.CredentialMachine:
		return packages.ScopeMachine, nil
	case config.CredentialOwn:
		return packages.ScopeWorkspace, nil
	default:
		return c.Scope, nil
	}
}

// scopeOf is Resolve with the refusal dropped, for the callers that only
// describe the shape. It never returns the shared answer it refused, so what
// they describe is what a run would do — minus the error, which Sharing
// reports.
func scopeOf(pkg string, c packages.Credential, prefer map[string]string) string {
	scope, _ := Resolve(pkg, c, prefer)
	return scope
}

// Account is one workspace a shared login is copied into.
type Account struct {
	Workspace string
	LinuxUser string
}

// Shared is one login obtained once and copied into the workspaces that use
// it.
type Shared struct {
	Declared
	// From is the master copy, under MachineDir. StoredAt, which Declared
	// carries, is where each account's tool looks for it.
	From string
	// Into is every workspace that declares the credential and did not opt
	// out. A workspace that opted out is not here, and that is the whole
	// point: the copy overwrites stored_at, so reaching a workspace that
	// logged in for itself would destroy that account on the next run.
	Into []Account
}

// Sharing lists the logins that are obtained once and copied, and where each
// copy goes.
//
// A credential nobody resolved to machine scope is not here at all: there is
// nothing to copy and nowhere to copy it.
func Sharing(plan packages.MachinePlan) ([]Shared, error) {
	var (
		out []Shared
		at  = map[string]int{}
	)

	for _, workspace := range plan.Workspaces {
		for _, found := range workspace.Ordered {
			for _, c := range found.Manifest.Credentials {
				if c.Kind != packages.KindLogin || c.StoredAt == "" {
					continue
				}
				scope, err := Resolve(found.Manifest.Name, c, workspace.Target.Credentials)
				if err != nil {
					return nil, fmt.Errorf("workspace %q: %w", workspace.Target.Name, err)
				}
				if scope != packages.ScopeMachine {
					continue
				}

				index, known := at[c.Name]
				if !known {
					index = len(out)
					at[c.Name] = index
					out = append(out, Shared{
						Declared: Declared{Credential: c, Package: found.Manifest.Name},
						From:     path.Join(MachineDir(c.Name), path.Base(c.StoredAt)),
					})
				}
				out[index].Into = append(out[index].Into, Account{
					Workspace: workspace.Target.Name,
					LinuxUser: workspace.Target.LinuxUser,
				})
			}
		}
	}
	return out, nil
}
