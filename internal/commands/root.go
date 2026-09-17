package commands

import (
	"fmt"
	"os"
)

var version = "dev"

// SetVersion records the version the binary was built with.
func SetVersion(v string) { version = v }

// Execute runs the CLI and turns an error into an exit code.
func Execute() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q, run \"devmachine help\"", args[0])
	}
}

func usage() {
	fmt.Println("devmachine — operate a personal development VPS")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  devmachine <command> [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  version   print the version")
	fmt.Println("  help      print this help")
}
