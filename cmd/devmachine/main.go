// Command devmachine operates a personal development VPS.
package main

import "github.com/adevmachine/cli/internal/commands"

// version is overridden at build time through -ldflags.
var version = "dev"

func main() {
	commands.SetVersion(version)
	commands.Execute()
}
