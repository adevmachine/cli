// Package expose renders the Caddy block a public hostname needs.
package expose

import (
	"fmt"
	"strings"
)

// Site is one public hostname pointing at one local port.
type Site struct {
	Host string // app.example.com
	Port int    // 8080
	// Workspace is whose port it is; empty for the machine's own.
	Workspace string
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
