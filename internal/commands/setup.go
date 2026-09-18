package commands

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newSetupCmd(opts *options) *cobra.Command {
	var force bool

	c := &cobra.Command{
		Use:   "setup",
		Short: "Write the configuration by answering a few questions",
		Long: "This version only writes the configuration. It does not touch a " +
			"machine: installing a key, hardening SSH and installing Ansible " +
			"come later.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			return runSetup(dir, cmd.InOrStdin(), cmd.OutOrStdout(), force)
		},
	}
	c.Flags().BoolVar(&force, "force", false, "overwrite a configuration that already exists")
	return c
}

// runSetup asks the questions and writes config.yml.
//
// It reads and writes through the streams it is given, so the whole flow is
// testable without a terminal.
func runSetup(dir string, in io.Reader, out io.Writer, force bool) error {
	path := filepath.Join(dir, config.FileName)
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("%s already exists: pass --force to overwrite it", path)
	}

	r := bufio.NewReader(in)

	machine, err := ask(r, out, "machine name", "main")
	if err != nil {
		return err
	}
	address, err := ask(r, out, "address (an IP, a hostname, or tailscale:<name>)", "")
	if err != nil {
		return err
	}
	if address == "" {
		return errors.New("an address is required: the CLI has nothing to reach without one")
	}
	admin, err := ask(r, out, "administrative login", config.DefaultAdminUser)
	if err != nil {
		return err
	}
	portText, err := ask(r, out, "SSH port", strconv.Itoa(config.DefaultPort))
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return fmt.Errorf("%q is not a port number", portText)
	}
	domain, err := ask(r, out, "domain (leave empty for none)", "")
	if err != nil {
		return err
	}

	cfg := config.Config{
		Machines: []config.Machine{{
			Name:  machine,
			Hosts: []config.Host{{Address: address}},
			User:  admin,
			Port:  port,
		}},
		Domain: domain,
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	body, err := yaml.Marshal(configFile{
		Machines: []machineFile{{
			Name:  machine,
			Hosts: []string{address},
			User:  admin,
			Port:  port,
		}},
		Domain: domain,
	})
	if err != nil {
		return fmt.Errorf("rendering the configuration: %w", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	fmt.Fprintf(out, "\nwrote %s\n", path)
	fmt.Fprintf(out, "Nothing on the machine has changed. Next: `devmachine doctor`.\n")
	return nil
}

// configFile is the shape written to disk. It is separate from config.Config so
// the file keeps its intended field order and omits what was left empty.
type configFile struct {
	Machines   []machineFile   `yaml:"machines"`
	Workspaces []workspaceFile `yaml:"workspaces,omitempty"`
	Domain     string          `yaml:"domain,omitempty"`
}

type machineFile struct {
	Name  string   `yaml:"name"`
	Hosts []string `yaml:"hosts"`
	User  string   `yaml:"user"`
	Port  int      `yaml:"port"`
	Key   string   `yaml:"key,omitempty"`
}

type workspaceFile struct {
	Name    string `yaml:"name"`
	Machine string `yaml:"machine,omitempty"`
	User    string `yaml:"user,omitempty"`
}

// ask prints a question and reads one line. An empty answer takes the default.
func ask(r *bufio.Reader, out io.Writer, question, fallback string) (string, error) {
	if fallback != "" {
		fmt.Fprintf(out, "%s [%s]: ", question, fallback)
	} else {
		fmt.Fprintf(out, "%s: ", question)
	}

	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return fallback, nil
		}
		return "", fmt.Errorf("reading the answer: %w", err)
	}

	answer := strings.TrimSpace(line)
	if answer == "" {
		return fallback, nil
	}
	return answer, nil
}
