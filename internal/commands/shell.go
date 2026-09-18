package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/remote"
	"github.com/spf13/cobra"
)

// Seams, so a test can see what would be launched without launching it.
var (
	realExecCommand = execCommand
	runInteractive  = realExecCommand
	realLookPath    = exec.LookPath
	lookPath        = realLookPath
)

func execCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// loadConfig resolves the directory, reads the configuration and judges it.
// Every command that touches the machine starts here.
func loadConfig(opts *options) (config.Config, error) {
	dir, _, err := config.Dir(opts.configDir)
	if err != nil {
		return config.Config{}, err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return cfg, err
	}
	return cfg, cfg.Validate()
}

// firstAddress is the address an interactive session should use. It is the
// same order Dial tries, without opening a connection first.
func firstAddress(m config.Machine) (string, error) {
	addresses, err := remote.Resolve(m)
	if err != nil {
		return "", err
	}
	return addresses[0], nil
}

func newSSHCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "ssh [workspace]",
		Short: "Open an interactive session in a workspace",
		Long: "Names a workspace to land in that environment, wherever it runs. " +
			"With no name it opens a session on the machine itself, as its admin.\n\n" +
			"The session runs through the system ssh, so the terminal, the agent " +
			"and tmux behave exactly as they do when you run ssh yourself.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return interactive(opts, "ssh", args)
		},
	}
}

func newMoshCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "mosh [workspace]",
		Short: "Open an interactive session over mosh",
		Long: "mosh survives a link that drops or roams, where ssh gives a " +
			"broken pipe. It needs mosh on both sides.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return interactive(opts, "mosh", args)
		},
	}
}

func interactive(opts *options, binary string, args []string) error {
	var name string
	if len(args) == 1 {
		name = args[0]
	}
	tgt, err := workspaceTarget(opts, name)
	if err != nil {
		return err
	}
	address, err := firstAddress(tgt.machine)
	if err != nil {
		return err
	}
	if _, err := lookPath(binary); err != nil {
		return fmt.Errorf("%s is not installed on this machine", binary)
	}

	m, user := tgt.machine, tgt.login()

	var argv []string
	switch binary {
	case "mosh":
		// mosh takes the remote ssh command as one string, which is where the
		// port and the key have to go.
		ssh := "ssh -p " + strconv.Itoa(m.Port)
		if m.Key != "" {
			ssh += " -i " + m.Key
		}
		argv = []string{"--ssh=" + ssh, user + "@" + address}
	default:
		argv = []string{"-p", strconv.Itoa(m.Port)}
		if m.Key != "" {
			argv = append(argv, "-i", m.Key, "-o", "IdentitiesOnly=yes")
		}
		argv = append(argv, user+"@"+address)
	}
	return runInteractive(binary, argv...)
}

func newRunCmd(opts *options) *cobra.Command {
	var workspace string

	c := &cobra.Command{
		Use:   "run <command>",
		Short: "Run one command and print its output",
		Long: "Runs as the machine's admin by default, or inside a workspace " +
			"with --workspace.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tgt, err := workspaceTarget(opts, workspace)
			if err != nil {
				return err
			}
			client, _, err := remote.Dial(cmd.Context(), tgt.machine, tgt.user)
			if err != nil {
				return err
			}
			defer client.Close()

			out, err := client.Run(cmd.Context(), args[0])
			// The output of a command that failed is usually the explanation,
			// so it is printed before the error is reported.
			if out != "" {
				cmd.Print(out)
			}
			return err
		},
	}
	c.Flags().StringVar(&workspace, "workspace", "", "run inside this workspace instead of as the machine's admin")
	return c
}
