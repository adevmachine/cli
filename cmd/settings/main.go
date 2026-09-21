// Command settings writes docs/reference/settings.md from the packages' own
// manifests.
//
// The page is the only place an operator can find out what is configurable, so
// it is generated rather than written: a page kept by hand beside the thing it
// describes is a page that drifts. Same argument as cmd/surface.
package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adevmachine/cli/internal/packages"
)

const preamble = `# Settings

Every variable the published packages accept, and what each one defaults to.

A setting is written ` + "`<package>.<name>`" + ` under a machine's or a workspace's
` + "`settings:`" + `, and it reaches the recipe as the variable it already reads.
**A setting is a default somebody overrode, not a new mechanism** — the recipe
cannot tell the difference and does not have to.

` + "```yaml" + `
machines:
  - name: main
    packages: [base, caddy]
    settings:
      base.timezone: Europe/Lisbon
      caddy.email: someone@example.com
` + "```" + `

Or without opening the file:

` + "```" + `
devmachine workspaces edit alice --set zsh.tmux_config=false
` + "```" + `

A setting for a package the target does not install is refused. A typo in a
package name would otherwise be silent: the value would reach nothing, the
recipe would keep its default, and the machine would not be what the
configuration says it is.

**This page is generated** by ` + "`make settings`" + `. Do not edit it by hand.

| Setting | What it does | Default |
| --- | --- | --- |
`

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: settings <packages directory>")
		os.Exit(1)
	}

	entries, err := os.ReadDir(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	var out strings.Builder
	out.WriteString(preamble)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		m, err := packages.ParseManifest(filepath.Join(os.Args[1], entry.Name()))
		if err != nil {
			continue
		}
		for _, key := range slices.Sorted(maps.Keys(m.Variables)) {
			v := m.Variables[key]
			fmt.Fprintf(&out, "| `%s.%s` | %s | %s |\n", m.Name, key, v.Summary, defaultOf(v.Default))
		}
	}

	if _, err := os.Stdout.WriteString(out.String()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// defaultOf renders a default so the table stays readable. An empty string is
// the common case and deserves to look deliberate rather than blank.
func defaultOf(value any) string {
	switch v := value.(type) {
	case nil:
		return "*(none)*"
	case string:
		if v == "" {
			return "*(empty)*"
		}
		return "`" + v + "`"
	default:
		return fmt.Sprintf("`%v`", v)
	}
}
