// Command surface prints the CLI's command paths, one per line.
//
// Its output is committed as SURFACE.txt, so a change to what the CLI offers
// shows up in a diff instead of passing unnoticed.
package main

import (
	"fmt"
	"os"

	"github.com/adevmachine/cli/internal/commands"
)

func main() {
	if err := commands.WriteSurface(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
