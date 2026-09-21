package provision

import (
	"fmt"
	"path"
	"strings"

	"github.com/adevmachine/cli/internal/credentials"
	"github.com/adevmachine/cli/internal/packages"
)

// credentialsTag runs the distribution of the shared logins on its own,
// without the packages that declared them.
const credentialsTag = "credentials"

// sharedLoopVar names the loop variable, for the reason loop_var is named
// everywhere else here: a role or a task with a loop of its own would rebind
// `item`.
const sharedLoopVar = "devmachine_shared"

// sharingTasks spread one login across the workspaces that share it.
//
// This is generated rather than written in a package because the CLI knows
// both halves and no package knows either: `stored_at` says where a tool keeps
// its session, and the configuration says which workspaces want the shared
// one. A package that wrote its own copy tasks would write them again, and
// slightly differently, for every shareable tool.
func sharingTasks(plan packages.MachinePlan) (string, error) {
	shared, err := credentials.Sharing(plan)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	for _, s := range shared {
		register := "devmachine_shared_" + ansibleName(s.Name)
		tags := fmt.Sprintf("[%s, %s]", s.Package, credentialsTag)

		// Nobody can automate a browser login, so a run before it has happened
		// is ordinary rather than a mistake. It skips, and the next run picks
		// the session up.
		fmt.Fprintf(&out, "    - name: look for the shared %s login\n", s.Name)
		fmt.Fprintf(&out, "      stat:\n        path: %q\n", s.From)
		fmt.Fprintf(&out, "      register: %s\n", register)
		out.WriteString("      check_mode: false\n")
		fmt.Fprintf(&out, "      tags: %s\n\n", tags)

		fmt.Fprintf(&out, "    - name: the directories the shared %s login lands in\n", s.Name)
		out.WriteString("      file:\n" +
			"        path: \"{{ " + sharedLoopVar + ".path }}\"\n" +
			"        state: directory\n" +
			"        owner: \"{{ " + sharedLoopVar + ".user }}\"\n" +
			"        group: \"{{ " + sharedLoopVar + ".user }}\"\n" +
			"        mode: \"0700\"\n")
		out.WriteString("      loop:\n")
		for _, account := range s.Into {
			// Every level, not just the last. Ansible's `file` creates the
			// parents it needs the way mkdir -p does — with the default owner
			// — so root would leave root-owned directories in a workspace's
			// home, and that workspace's own tools would start failing for a
			// reason nobody would connect to this.
			for _, dir := range accountDirs(account.LinuxUser, s.StoredAt) {
				fmt.Fprintf(&out, "        - {user: %q, path: %q}\n", account.LinuxUser, dir)
			}
		}
		out.WriteString(loopControl())
		fmt.Fprintf(&out, "      when: %s.stat.exists\n", register)
		fmt.Fprintf(&out, "      tags: %s\n\n", tags)

		fmt.Fprintf(&out, "    - name: the shared %s login, for the workspaces that want it\n", s.Name)
		fmt.Fprintf(&out, "      copy:\n        src: %q\n", s.From)
		out.WriteString("        dest: \"{{ " + sharedLoopVar + ".path }}\"\n" +
			"        remote_src: true\n" +
			"        owner: \"{{ " + sharedLoopVar + ".user }}\"\n" +
			"        group: \"{{ " + sharedLoopVar + ".user }}\"\n" +
			"        mode: \"0600\"\n")
		out.WriteString("      loop:\n")
		for _, account := range s.Into {
			fmt.Fprintf(&out, "        - {user: %q, path: %q}\n",
				account.LinuxUser, accountPath(account.LinuxUser, s.StoredAt))
		}
		out.WriteString(loopControl())
		fmt.Fprintf(&out, "      when: %s.stat.exists\n", register)
		fmt.Fprintf(&out, "      tags: %s\n\n", tags)
	}
	return out.String(), nil
}

func loopControl() string {
	return "      loop_control:\n        loop_var: " + sharedLoopVar + "\n"
}

// accountPath turns a package's `~/...` into one account's copy of it.
//
// `~alice` rather than a home worked out here: the machine is the only thing
// that knows where an account's home really is, and Ansible expands a named
// home on the machine.
func accountPath(user, storedAt string) string {
	if rest, found := strings.CutPrefix(storedAt, "~/"); found {
		return "~" + user + "/" + rest
	}
	return storedAt
}

// accountDirs lists the directories one account needs on the way to its copy,
// outermost first.
//
// The home itself is never among them: it belongs to the package that made the
// account, and it is 0750 rather than 0700 so that the CLI can reach it.
func accountDirs(user, storedAt string) []string {
	root := "~" + user + "/"
	rest, inHome := strings.CutPrefix(storedAt, "~/")
	if !inHome {
		root, rest = "/", strings.TrimPrefix(storedAt, "/")
	}

	dir := path.Dir(rest)
	if dir == "." || dir == "/" {
		return nil
	}

	parts := strings.Split(dir, "/")
	out := make([]string, 0, len(parts))
	for i := range parts {
		out = append(out, root+strings.Join(parts[:i+1], "/"))
	}
	return out
}

// ansibleName turns a credential's name into something a register accepts.
func ansibleName(name string) string {
	return strings.NewReplacer("-", "_", ".", "_").Replace(name)
}
