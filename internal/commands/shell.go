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
func firstAddress(cfg config.Config) (string, error) {
	addresses, err := remote.Resolve(cfg)
	if err != nil {
		return "", err
	}
	return addresses[0], nil
}

func newSSHCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "ssh [user]",
		Short: "Open an interactive session on the machine",
		Long: "Opens a session with the system ssh, so the terminal, the agent " +
			"and tmux behave exactly as they do when you run ssh yourself.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return interactive(opts, "ssh", args)
		},
	}
}

func newMoshCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "mosh [user]",
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
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}
	address, err := firstAddress(cfg)
	if err != nil {
		return err
	}
	if _, err := lookPath(binary); err != nil {
		return fmt.Errorf("%s is not installed on this machine", binary)
	}

	user := cfg.User
	if len(args) == 1 {
		user = args[0]
	}

	var argv []string
	switch binary {
	case "mosh":
		// mosh takes the remote ssh command as one string, which is where the
		// port and the key have to go.
		ssh := "ssh -p " + strconv.Itoa(cfg.Port)
		if cfg.Key != "" {
			ssh += " -i " + cfg.Key
		}
		argv = []string{"--ssh=" + ssh, user + "@" + address}
	default:
		argv = []string{"-p", strconv.Itoa(cfg.Port)}
		if cfg.Key != "" {
			argv = append(argv, "-i", cfg.Key, "-o", "IdentitiesOnly=yes")
		}
		argv = append(argv, user+"@"+address)
	}
	return runInteractive(binary, argv...)
}

func newRunCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "run <command>",
		Short: "Run one command on the machine and print its output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			client, _, err := remote.Dial(cmd.Context(), cfg)
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
}
