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

// routeFiles is what routeTasks makes true for each workspace on the plan, in
// plan order: its routes file written, or removed when it has no route left.
func routeFiles(plan packages.MachinePlan) []expose.FileChange {
	byWorkspace := routesByWorkspace(plan)
	out := make([]expose.FileChange, 0, len(plan.Workspaces))
	for _, w := range plan.Workspaces {
		name := w.Target.Name
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
		out = append(out, change)
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
	for i, w := range plan.Workspaces {
		if w.Target.Name == workspace {
			return routeFiles(plan)[i], nil
		}
	}
	return expose.FileChange{}, fmt.Errorf("workspace %q is not on %s", workspace, plan.Machine.Name)
}
