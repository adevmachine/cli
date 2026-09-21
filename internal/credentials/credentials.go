// Package credentials works out what the packages on a machine cannot run
// without, where each of those things lands, and whether it is there.
//
// A package declares a credential and how it is obtained; the operator supplies
// only a value, or sits through a login. Nothing here knows what `gh` or
// `claude` are.
package credentials

import (
	"path"

	"github.com/adevmachine/cli/internal/packages"
)

// root is the one directory every machine-scoped credential lives under.
//
// It is not invented here: the operator repository has been keeping a shared
// `gh` session in /etc/devmachine/gh since long before this CLI existed. One
// directory per credential, root owned, copied into the workspaces that need it.
const root = "/etc/devmachine"

// MachineDir is where a machine-scoped credential lives.
func MachineDir(name string) string { return path.Join(root, name) }

// EnvFile is the file a value is delivered into, and it is a published
// contract: a provider's entrypoint sources this exact path before it runs, so
// one published today still works against a CLI released years later. It does
// not change.
func EnvFile(name string) string { return path.Join(MachineDir(name), "env") }

// Declared is one credential, the package that asked for it, and the workspace
// it belongs to.
type Declared struct {
	packages.Credential
	Package string
	// Workspace is empty for a machine-scoped credential.
	Workspace string
	// LinuxUser is the account that owns a workspace-scoped credential. Only
	// the CLI knows whose home a package's `~` means.
	LinuxUser string
}

// Key names a credential in a report, a lookup and a command argument.
func Key(d Declared) string {
	if d.Workspace == "" {
		return d.Name
	}
	return d.Workspace + "/" + d.Name
}

// Wanted lists every credential the packages on this machine declare.
//
// A machine-scoped one appears once however many workspaces ask for it: one
// `gh` session serves them all. A workspace-scoped one appears once per
// workspace, because accounts differ between workspaces and there is no master
// session to copy.
func Wanted(plan packages.MachinePlan) []Declared {
	var (
		out  []Declared
		seen = map[string]bool{}
	)

	add := func(d Declared) {
		key := Key(d)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, d)
	}

	// A machine has no workspace, so a credential declared by a machine
	// package belongs to the machine whatever scope it claims.
	for _, found := range plan.OnMachine.Ordered {
		for _, c := range found.Manifest.Credentials {
			add(Declared{Credential: c, Package: found.Manifest.Name})
		}
	}
	// The scope is what the operator settled on, not what the package
	// recommended: a workspace that keeps its own account has a credential of
	// its own to obtain, and the shared one is not it.
	for _, workspace := range plan.Workspaces {
		for _, found := range workspace.Ordered {
			for _, c := range found.Manifest.Credentials {
				if scopeOf(found.Manifest.Name, c, workspace.Target.Credentials) != packages.ScopeMachine {
					continue
				}
				add(Declared{Credential: c, Package: found.Manifest.Name})
			}
		}
	}

	for _, workspace := range plan.Workspaces {
		for _, found := range workspace.Ordered {
			for _, c := range found.Manifest.Credentials {
				if scopeOf(found.Manifest.Name, c, workspace.Target.Credentials) == packages.ScopeMachine {
					continue
				}
				add(Declared{
					Credential: c,
					Package:    found.Manifest.Name,
					Workspace:  workspace.Target.Name,
					LinuxUser:  workspace.Target.LinuxUser,
				})
			}
		}
	}
	return out
}
