package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mydevmachine/cli/internal/config"
)

// controlPersist is how long a multiplexed master connection lingers after
// the process that opened it exits. It is what lets a poll a few seconds
// later reuse the connection instead of paying for a new handshake.
const controlPersist = "5m"

// Seams, so a test can drive DialMux without a network or a real cache
// directory.
var (
	userCacheDir = os.UserCacheDir
	dialFallback = Dial
)

// DialMux is Dial for a short, repeated call such as `run`: it hands
// commands to the system OpenSSH client with connection multiplexing, so a
// second process a few seconds later reuses the first one's authenticated
// connection instead of paying for a new TCP handshake, key exchange and
// authentication.
//
// It falls back to Dial when the system ssh binary is not on the machine.
func DialMux(ctx context.Context, m config.Machine, user string) (Client, string, error) {
	if m.Self {
		return &localClient{}, SelfAddress, nil
	}
	sshPath, err := lookPath("ssh")
	if err != nil {
		return dialFallback(ctx, m, user)
	}
	addresses, err := Resolve(m)
	if err != nil {
		return nil, "", err
	}
	if user == "" {
		user = m.User
	}
	controlDir, err := controlPathDir()
	if err != nil {
		return nil, "", err
	}

	args := append(StrictSSHArgs(m), muxArgs(controlDir)...)
	client := &muxClient{
		sshPath:   sshPath,
		args:      args,
		addresses: addresses,
		user:      user,
		machine:   m.Name,
	}
	return client, addresses[0], nil
}

// controlPathDir is where every ControlMaster socket lives, one directory
// shared by every machine: ssh's own %C in ControlPath hashes the
// host/port/user into the filename, which is what keeps each socket path
// under the ~104-byte Unix socket limit.
func controlPathDir() (string, error) {
	cache, err := userCacheDir()
	if err != nil {
		return "", fmt.Errorf("finding the cache directory: %w", err)
	}
	dir := filepath.Join(cache, "devmachine", "cm")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	return dir, nil
}

func muxArgs(controlDir string) []string {
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + filepath.Join(controlDir, "%C"),
		"-o", "ControlPersist=" + controlPersist,
		"-o", "BatchMode=yes",
	}
}

// muxClient runs commands through the system ssh binary, one process per
// command, relying on ControlMaster/ControlPersist to make every process
// after the first reuse an already-authenticated connection.
type muxClient struct {
	sshPath   string
	args      []string
	addresses []string
	user      string
	machine   string
}

func (c *muxClient) Run(ctx context.Context, command string) (string, error) {
	var stdout bytes.Buffer
	err := c.exec(ctx, command, nil, &stdout, nil)
	return stdout.String(), err
}

func (c *muxClient) RunInput(ctx context.Context, command string, stdin io.Reader) (string, error) {
	var stdout bytes.Buffer
	err := c.exec(ctx, command, stdin, &stdout, nil)
	return stdout.String(), err
}

func (c *muxClient) Stream(ctx context.Context, command string, stdout, stderr io.Writer) error {
	return c.exec(ctx, command, nil, stdout, stderr)
}

func (c *muxClient) Upload(context.Context, string, io.Reader) error {
	return errors.New("uploading a directory needs the programmatic SSH client: the multiplexed client only runs commands")
}

func (c *muxClient) Close() error { return nil }

// exec runs command over ssh, trying each resolved address in turn.
//
// OpenSSH's own ssh distinguishes the two failure shapes this needs: exit
// 255 is ssh's code for "never reached the machine" (bad address, refused
// connection, failed handshake), so that address is dropped in favour of the
// next one. Any other exit status is the remote command's own, and is
// returned the same way sshClient.Run returns one: the error wraps it, and
// stdout still holds whatever the command printed before it failed.
func (c *muxClient) exec(ctx context.Context, command string, stdin io.Reader, stdout, stderr io.Writer) error {
	var failures []string
	for _, address := range c.addresses {
		argv := append(append([]string{}, c.args...), c.user+"@"+address, command)
		cmd := exec.CommandContext(ctx, c.sshPath, argv...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout

		var errBuf bytes.Buffer
		if stderr != nil {
			cmd.Stderr = io.MultiWriter(stderr, &errBuf)
		} else {
			cmd.Stderr = &errBuf
		}

		runErr := cmd.Run()
		if runErr == nil {
			return nil
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 255 {
			failures = append(failures, fmt.Sprintf("%s (%s)", address, strings.TrimSpace(errBuf.String())))
			continue
		}
		return fmt.Errorf("running %q: %w", command, runErr)
	}
	return fmt.Errorf("machine %q: no address answered: %s", c.machine, strings.Join(failures, "; "))
}
