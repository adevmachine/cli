package credentials

import (
	"context"
	"io"
	"os"
	"strconv"
	"testing"

	"github.com/adevmachine/cli/internal/config"
)

// recordingClient keeps what was asked of the machine, so a test can read the
// shell a delivery writes without needing a machine to run it on.
type recordingClient struct {
	commands []string
	inputs   []string
	output   map[string]string
	err      error
}

func (c *recordingClient) Run(_ context.Context, command string) (string, error) {
	c.commands = append(c.commands, command)
	return c.output[command], c.err
}

func (c *recordingClient) RunInput(ctx context.Context, command string, stdin io.Reader) (string, error) {
	body, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	c.inputs = append(c.inputs, string(body))
	return c.Run(ctx, command)
}

func (c *recordingClient) Stream(ctx context.Context, command string, _, _ io.Writer) error {
	_, err := c.Run(ctx, command)
	return err
}

func (c *recordingClient) Upload(context.Context, string, io.Reader) error { return nil }
func (c *recordingClient) Close() error                                    { return nil }

// testMachine describes the throwaway VPS from scripts/fake-vps.sh. Without it
// the integration tests skip, so `go test ./...` still works where there is no
// VM.
func testMachine(t *testing.T) config.Machine {
	t.Helper()

	host := os.Getenv("DEVMACHINE_TEST_HOST")
	port := os.Getenv("DEVMACHINE_TEST_PORT")
	key := os.Getenv("DEVMACHINE_TEST_KEY")
	if host == "" || port == "" || key == "" {
		t.Skip("no test VPS: run `eval \"$(scripts/fake-vps.sh env)\"` first")
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("DEVMACHINE_TEST_PORT is not a number: %v", err)
	}

	user := os.Getenv("DEVMACHINE_TEST_USER")
	if user == "" {
		user = "root"
	}
	return config.Machine{
		Name:  "sandbox",
		Hosts: []config.Host{{Address: host}},
		User:  user,
		Port:  n,
		Key:   key,
	}
}
