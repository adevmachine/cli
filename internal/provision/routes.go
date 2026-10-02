package provision

import (
	"fmt"
	"path"

	"github.com/mydevmachine/devmachine/internal/expose"
	"github.com/mydevmachine/devmachine/internal/packages"
)

// A routes file is owned and permissioned the same way whether sync or
// `expose` writes it, so neither sees the other's file as a change.
const (
	routeFileOwner = "root"
	routeFileGroup = "root"
	routeFileMode  = "0644"
)

// routeFile is one workspace's routes file on this machine.
type routeFile struct {
	workspace string
	change    expose.FileChange
	// unresolved is set when a route here comes from a machine whose address
	// is not known: the file is then left as it is, neither written nor
	// removed, so the site keeps answering the way it did.
	unresolved bool
}

// routeFiles is what routeTasks makes true for each workspace, in plan order
// and then the workspaces elsewhere: its routes file written, or removed when
// it has no route here.
func routeFiles(plan packages.MachinePlan) []routeFile {
	byWorkspace := routesByWorkspace(plan)
	unresolved := map[string]bool{}
	for _, r := range UnresolvedRoutes(plan) {
		unresolved[r.Workspace] = true
	}
	names := make([]string, 0, len(plan.Workspaces)+len(plan.OtherWorkspaces))
	for _, w := range plan.Workspaces {
		names = append(names, w.Target.Name)
	}
	names = append(names, plan.OtherWorkspaces...)

	out := make([]routeFile, 0, len(names))
	for _, name := range names {
		change := expose.FileChange{
			Path:  path.Join(plan.SitesDir, expose.WorkspaceFileName(name)),
			Owner: routeFileOwner,
			Group: routeFileGroup,
			Mode:  routeFileMode,
		}
		if sites := byWorkspace[name]; len(sites) > 0 {
			change.Content = []byte(expose.RenderWorkspace(name, sites))
			for _, s := range sites {
				change.Stale = append(change.Stale, path.Join(plan.SitesDir, expose.FileName(s)))
			}
		}
		out = append(out, routeFile{workspace: name, change: change, unresolved: unresolved[name]})
	}
	return out
}

// UnresolvedRoutes are the routes from another machine whose address the
// caller could not find.
func UnresolvedRoutes(plan packages.MachinePlan) []packages.Route {
	var out []packages.Route
	for _, r := range plan.Routes {
		if r.From != "" && r.Upstream == "" {
			out = append(out, r)
		}
	}
	return out
}

// WorkspaceRoutes is the one workspace's share of what sync does to Caddy's
// sites directory, so `expose` can apply a route without running the play and
// leave sync nothing to change.
func WorkspaceRoutes(plan packages.MachinePlan, workspace string) (expose.FileChange, error) {
	if plan.SitesDir == "" {
		return expose.FileChange{}, fmt.Errorf("caddy is not on %s", plan.Machine.Name)
	}
	for _, f := range routeFiles(plan) {
		if f.workspace != workspace {
			continue
		}
		if f.unresolved {
			return expose.FileChange{}, fmt.Errorf("the address of %s is not known from here", fromOf(plan, workspace))
		}
		return f.change, nil
	}
	return expose.FileChange{}, fmt.Errorf("workspace %q is not on %s", workspace, plan.Machine.Name)
}

func fromOf(plan packages.MachinePlan, workspace string) string {
	for _, r := range UnresolvedRoutes(plan) {
		if r.Workspace == workspace {
			return r.From
		}
	}
	return ""
}
