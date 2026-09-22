package commands

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/adevmachine/cli/internal/aliases"
	"github.com/spf13/cobra"
)

// hostOS is the seam a test replaces, so `machine setup` can be driven as if
// it ran on Linux without leaving the Mac that runs the test.
var hostOS = runtime.GOOS

const (
	machinePass = "pass"
	machineWarn = "warn"
	machineFail = "fail"
)

// machineCheck is one thing `machine doctor` looked at on this computer.
type machineCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func machineChecksOK(checks []machineCheck) bool {
	for _, c := range checks {
		if c.Status == machineFail {
			return false
		}
	}
	return true
}

func newMachineCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machine",
		Short: "The operator's own computer — the one thing left to set up by hand",
	}
	cmd.AddCommand(newMachineDoctorCmd(opts), newMachineSetupCmd(opts))
	return cmd
}

// binaryChecks looks at `ssh` and `mosh`. A missing `mosh` is a warning, not
// a failure: `ssh` alone is enough to reach a machine, and reporting a nicety
// as broken makes a working computer look like it is not.
func binaryChecks() []machineCheck {
	var out []machineCheck
	if _, err := lookPath("ssh"); err != nil {
		out = append(out, machineCheck{Name: "ssh", Status: machineFail, Detail: "not on PATH"})
	} else {
		out = append(out, machineCheck{Name: "ssh", Status: machinePass})
	}
	if _, err := lookPath("mosh"); err != nil {
		out = append(out, machineCheck{Name: "mosh", Status: machineWarn,
			Detail: "not on PATH; ssh is enough without it"})
	} else {
		out = append(out, machineCheck{Name: "mosh", Status: machinePass})
	}
	return out
}

// agentCheck looks for either an SSH agent or a key named in the
// configuration — one of the two is what every dial in this CLI needs.
func agentCheck(opts *options) machineCheck {
	if os.Getenv("SSH_AUTH_SOCK") != "" {
		return machineCheck{Name: "ssh-agent", Status: machinePass, Detail: "an agent is running"}
	}
	if cfg, err := loadConfig(opts); err == nil {
		for _, m := range cfg.Machines {
			if m.Key != "" {
				return machineCheck{Name: "ssh-agent", Status: machinePass, Detail: "key: " + m.Key}
			}
		}
	}
	return machineCheck{Name: "ssh-agent", Status: machineFail,
		Detail: "no SSH agent and no key in the configuration"}
}

// aliasesCheck says whether ~/.ssh/config holds the devmachine block, and
// whether it still matches what the configuration would write today. A
// workspace added after the last `aliases --write` is not reachable by name,
// and nothing else would tell you that.
func aliasesCheck(opts *options) machineCheck {
	cfg, err := loadConfig(opts)
	if err != nil {
		return machineCheck{Name: "aliases", Status: machineWarn, Detail: "no configuration to check against"}
	}
	want, err := aliases.Render(cfg)
	if err != nil {
		return machineCheck{Name: "aliases", Status: machineWarn, Detail: err.Error()}
	}
	path, err := aliases.DefaultPath()
	if err != nil {
		return machineCheck{Name: "aliases", Status: machineWarn, Detail: err.Error()}
	}

	notWritten := machineCheck{Name: "aliases", Status: machineFail,
		Detail: "not written yet: run `devmachine aliases --write`"}
	if len(cfg.Workspaces) == 0 {
		notWritten = machineCheck{Name: "aliases", Status: machinePass, Detail: "no workspace configured"}
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return notWritten
	}
	current := string(body)
	start := strings.Index(current, aliases.Begin)
	stop := strings.LastIndex(current, aliases.End)
	if start < 0 || stop <= start {
		return notWritten
	}

	existing := current[start+len(aliases.Begin) : stop]
	if strings.TrimSpace(existing) != strings.TrimSpace(want) {
		return machineCheck{Name: "aliases", Status: machineFail,
			Detail: "stale: a workspace changed since the last `devmachine aliases --write`"}
	}
	return machineCheck{Name: "aliases", Status: machinePass}
}

func machineDoctorChecks(opts *options) []machineCheck {
	checks := binaryChecks()
	checks = append(checks, agentCheck(opts), aliasesCheck(opts))
	return checks
}

func newMachineDoctorCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Say what is missing on this computer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := machineDoctorChecks(opts)

			if opts.format == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), struct {
					Checks []machineCheck `json:"checks"`
					OK     bool           `json:"ok"`
				}{checks, machineChecksOK(checks)}); err != nil {
					return err
				}
			} else {
				width := 10
				for _, c := range checks {
					width = max(width, len(c.Name))
				}
				for _, c := range checks {
					cmd.Printf("%-4s  %-*s  %s\n", c.Status, width, c.Name, c.Detail)
				}
			}

			if !machineChecksOK(checks) {
				return errors.New("some checks failed")
			}
			return nil
		},
	}
}

func newMachineSetupCmd(_ *options) *cobra.Command {
	var yes bool

	c := &cobra.Command{
		Use:   "setup",
		Short: "Install what this computer is missing to operate a machine",
		Long: "Installs `ssh` and `mosh` through Homebrew, on a Mac. On Linux " +
			"it says what to install rather than guessing a package manager.\n\n" +
			"It does not install an editor, shell plugins or language runtimes " +
			"— those are one person's taste, and taste stays in that person's " +
			"own configuration.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var missing []string
			for _, tool := range []string{"ssh", "mosh"} {
				if _, err := lookPath(tool); err != nil {
					missing = append(missing, tool)
				}
			}
			if len(missing) == 0 {
				cmd.Println("ssh and mosh are already on PATH. Nothing to install.")
				return nil
			}

			if hostOS != "darwin" {
				cmd.Printf("Install these yourself: %s. This only knows Homebrew, on a Mac.\n",
					strings.Join(missing, ", "))
				return nil
			}

			if _, err := lookPath("brew"); err != nil {
				return errors.New("homebrew is not installed: get it from https://brew.sh, then run this again")
			}
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(),
					"Install "+strings.Join(missing, ", ")+" with Homebrew?")
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}
			for _, tool := range missing {
				if err := runInteractive("brew", "install", tool); err != nil {
					return fmt.Errorf("installing %s: %w", tool, err)
				}
			}
			cmd.Printf("installed: %s\n", strings.Join(missing, ", "))
			return nil
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "install without asking")
	return c
}
