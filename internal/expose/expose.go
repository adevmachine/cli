// Package expose renders the Caddy block a public hostname needs.
package expose

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Site is one public hostname pointing at one local port.
type Site struct {
	Host string // app.example.com
	Port int    // 8080
	// Workspace is whose port it is; empty for the machine's own.
	Workspace string
	// Upstream is the address of the machine the port is on, when that is
	// not the machine serving the site; empty means this machine.
	Upstream string
}

const loopback = "127.0.0.1"

func (s Site) target() string {
	address := s.Upstream
	if address == "" {
		address = loopback
	}
	return net.JoinHostPort(address, strconv.Itoa(s.Port))
}

// Render returns the Caddy block for a site.
func Render(s Site) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Written by `devmachine expose`. Every site is a file of its own; this one\n")
	fmt.Fprintf(&b, "# belongs to %s.\n", s.Host)
	if s.Workspace != "" {
		fmt.Fprintf(&b, "# workspace: %s\n", s.Workspace)
	}
	fmt.Fprintf(&b, "%s {\n", s.Host)
	fmt.Fprintf(&b, "\treverse_proxy 127.0.0.1:%d\n", s.Port)
	fmt.Fprintf(&b, "}\n")
	return b.String()
}

// FileName is where the block is written inside caddy's sites.d.
//
// It returns "" for a host that is not a plain domain: one with a "/", a
// "..", or nothing in it. The host arrives from a command line and becomes a
// file name on a machine, and nothing about that is safe by accident.
func FileName(s Site) string {
	if s.Host == "" || strings.Contains(s.Host, "/") || strings.Contains(s.Host, "..") {
		return ""
	}
	return s.Host + ".caddy"
}

// RenderWorkspace is every route of one workspace in one file. One file per
// workspace, so removing a workspace's last route removes a file and not a
// line inside somebody else's.
func RenderWorkspace(workspace string, sites []Site) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Written by `devmachine sync` from config.yml. Edit the configuration and\n")
	fmt.Fprintf(&b, "# run `devmachine sync`; a change made here is undone by the next run.\n")
	fmt.Fprintf(&b, "# workspace: %s\n", workspace)
	for _, s := range sites {
		fmt.Fprintf(&b, "\n%s {\n\treverse_proxy %s\n}\n", s.Host, s.target())
	}
	return b.String()
}

// WorkspaceFileName is where a workspace's routes live inside caddy's sites.d.
func WorkspaceFileName(workspace string) string {
	return workspace + "-routes.caddy"
}

var (
	reBlock     = regexp.MustCompile(`^([^\s{#]+)\s*\{`)
	reProxy     = regexp.MustCompile(`reverse_proxy\s+(\S+)`)
	reWorkspace = regexp.MustCompile(`^#\s*workspace:\s*(\S+)`)
)

// Parse reads every site block in one file. A `# workspace:` comment names
// the owner of every block after it, which is how both the file this package
// writes and the one-host file the old `expose` wrote are read.
func Parse(content string) []Site {
	var (
		out       []Site
		workspace string
		current   *Site
	)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if m := reWorkspace.FindStringSubmatch(line); m != nil {
			workspace = m[1]
			continue
		}
		if m := reBlock.FindStringSubmatch(line); m != nil {
			out = append(out, Site{Host: m[1], Workspace: workspace})
			current = &out[len(out)-1]
			continue
		}
		if m := reProxy.FindStringSubmatch(line); m != nil && current != nil {
			current.Upstream, current.Port = splitTarget(m[1])
		}
	}
	return out
}

// splitTarget reads `address:port` back. A target that is not one, such as a
// URL written by hand, reads as port 0, which `list` reports as differing.
func splitTarget(target string) (string, int) {
	address, port, err := net.SplitHostPort(target)
	if err != nil {
		return "", 0
	}
	n, _ := strconv.Atoi(port)
	if address == loopback {
		address = ""
	}
	return address, n
}

// FileChange is what one routes file in caddy's sites directory must become.
type FileChange struct {
	// Path is the file in the sites directory.
	Path string
	// Content is nil when the file goes away.
	Content []byte
	// Stale are files removed with it: the one-host files the old `expose`
	// wrote for a host this file now owns, since Caddy refuses a host twice.
	Stale []string
	Owner string
	Group string
	Mode  string
}
