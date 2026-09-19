// Package remote reaches the machine over SSH.
//
// Anything programmatic goes through x/crypto/ssh, so the CLI controls which
// authentication method it offers. That matters: an agent holding many keys
// makes a server cut the connection at MaxAuthTries before the method that
// would have worked is ever tried.
//
// Handing the terminal to a human is a different job and belongs to the system
// ssh and mosh binaries, not here.
package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/adevmachine/cli/internal/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// tailscalePrefix marks an address that something closer to the network has to
// resolve. The CLI knows the prefix, not the product: a host entry is either a
// literal or a name a resolver understands.
const tailscalePrefix = "tailscale:"

// dialTimeout is short because these addresses are tried in order: a long wait
// on a dead path delays the fallback that works.
const dialTimeout = 8 * time.Second

// Seams, so the tests can drive every branch without a network.
var (
	realLookPath    = exec.LookPath
	lookPath        = realLookPath
	realTailscaleIP = tailscaleIPFromStatus
	tailscaleIP     = realTailscaleIP
)

// Client runs commands on the machine.
type Client interface {
	Run(ctx context.Context, command string) (string, error)
	// Stream runs a command with its output going to the writers as it
	// arrives, and returns the exit status.
	Stream(ctx context.Context, command string, stdout, stderr io.Writer) error
	// Upload extracts a gzipped tar into a directory on the machine.
	Upload(ctx context.Context, dir string, tarball io.Reader) error
	Close() error
}

// Resolve turns the configured hosts into addresses to try, in order.
//
// An address that cannot be resolved is dropped rather than fatal: the entries
// after it are the fallbacks it exists for. Only an empty result is an error,
// and it says what was dropped.
func Resolve(m config.Machine) ([]string, error) {
	var (
		out     []string
		dropped []string
	)

	for _, h := range m.Hosts {
		name, isTailscale := strings.CutPrefix(h.Address, tailscalePrefix)
		if !isTailscale {
			out = append(out, h.Address)
			continue
		}
		if _, err := lookPath("tailscale"); err != nil {
			dropped = append(dropped, fmt.Sprintf("%s (tailscale is not installed)", h.Address))
			continue
		}
		ip, err := tailscaleIP(name)
		if err != nil {
			dropped = append(dropped, fmt.Sprintf("%s (%v)", h.Address, err))
			continue
		}
		out = append(out, ip)
	}

	if len(out) == 0 {
		if len(dropped) > 0 {
			return nil, fmt.Errorf("no address left to try: %s", strings.Join(dropped, "; "))
		}
		return nil, fmt.Errorf("machine %q has no address", m.Name)
	}
	return out, nil
}

func tailscaleIPFromStatus(name string) (string, error) {
	body, err := exec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return "", fmt.Errorf("asking tailscale for its status: %w", err)
	}

	var status struct {
		Self *peer           `json:"Self"`
		Peer map[string]peer `json:"Peer"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return "", fmt.Errorf("reading the tailscale status: %w", err)
	}

	all := make([]peer, 0, len(status.Peer)+1)
	if status.Self != nil {
		all = append(all, *status.Self)
	}
	for _, p := range status.Peer {
		all = append(all, p)
	}
	for _, p := range all {
		if p.HostName == name && len(p.TailscaleIPs) > 0 {
			return p.TailscaleIPs[0], nil
		}
	}
	return "", fmt.Errorf("no machine named %q on the tailnet", name)
}

type peer struct {
	HostName     string   `json:"HostName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
}

// Dial tries each of a machine's addresses in order and returns the first
// client that answered, along with the address it answered on.
//
// user is who to log in as. Empty means the machine's administrative login;
// a workspace passes its own Linux account instead.
func Dial(ctx context.Context, m config.Machine, user string) (Client, string, error) {
	addresses, err := Resolve(m)
	if err != nil {
		return nil, "", err
	}
	auth, err := authMethods(m)
	if err != nil {
		return nil, "", err
	}
	if user == "" {
		user = m.User
	}

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: auth,
		// The CLI reaches a machine the operator already owns, over a path
		// they chose. Pinning a host key would need a store the CLI does not
		// have yet; that belongs with the first-contact bootstrap, which is
		// where trust is actually established.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         dialTimeout,
	}

	var (
		failures []string
		authErr  error
	)
	for _, address := range addresses {
		target := net.JoinHostPort(address, fmt.Sprint(m.Port))
		conn, err := dialContext(ctx, target, cfg)
		if err == nil {
			return &sshClient{conn: conn}, address, nil
		}
		// A machine that answers and then refuses the login is a different
		// problem from one that never answered, and the fix is different too.
		// Reporting both as "nothing answered" sends people to check the
		// network when the account or the key is what is wrong.
		if isAuthFailure(err) && authErr == nil {
			authErr = fmt.Errorf("machine %q answered on %s but refused the login for %q: %w",
				m.Name, address, user, err)
		}
		failures = append(failures, fmt.Sprintf("%s (%v)", address, err))
	}
	if authErr != nil {
		return nil, "", authErr
	}
	return nil, "", fmt.Errorf("machine %q: no address answered: %s", m.Name, strings.Join(failures, "; "))
}

// isAuthFailure reports whether the connection was made and the login was
// then refused.
func isAuthFailure(err error) bool {
	return strings.Contains(err.Error(), "unable to authenticate") ||
		strings.Contains(err.Error(), "no supported methods remain")
}

func dialContext(ctx context.Context, target string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	d := net.Dialer{Timeout: cfg.Timeout}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, target, cfg)
	if err != nil {
		// The handshake already failed; whether the socket closed cleanly
		// changes nothing anyone can act on.
		_ = conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

// authMethods offers the key from the configuration when there is one, and the
// agent otherwise. Never both: offering everything is what trips MaxAuthTries.
func authMethods(m config.Machine) ([]ssh.AuthMethod, error) {
	if m.Key != "" {
		body, err := os.ReadFile(m.Key)
		if err != nil {
			return nil, fmt.Errorf("reading the private key %s: %w", m.Key, err)
		}
		signer, err := ssh.ParsePrivateKey(body)
		if err != nil {
			return nil, fmt.Errorf("parsing the private key %s: %w", m.Key, err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}

	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, fmt.Errorf("no key in the configuration and no SSH agent: set `key` in config.yml or start an agent")
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("reaching the SSH agent: %w", err)
	}
	return []ssh.AuthMethod{ssh.PublicKeysCallback(agent.NewClient(conn).Signers)}, nil
}

type sshClient struct {
	conn *ssh.Client
}

// Run executes one command and returns its standard output.
func (c *sshClient) Run(ctx context.Context, command string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("opening a session: %w", err)
	}
	defer session.Close()

	stop := c.closeOnCancel(ctx, session)
	defer stop()

	out, err := session.Output(command)
	if err != nil {
		return string(out), fmt.Errorf("running %q: %w", command, err)
	}
	return string(out), nil
}

// Stream executes one command with its output reaching the writers as it
// arrives.
//
// Collecting the output and printing it at the end makes a long run look stuck
// when it is not, which is exactly what a provisioning run is.
func (c *sshClient) Stream(ctx context.Context, command string, stdout, stderr io.Writer) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("opening a session: %w", err)
	}
	defer session.Close()

	stop := c.closeOnCancel(ctx, session)
	defer stop()

	session.Stdout = stdout
	session.Stderr = stderr
	if err := session.Run(command); err != nil {
		return fmt.Errorf("running %q: %w", command, err)
	}
	return nil
}

// Upload extracts a gzipped tar into a directory on the machine.
//
// One session carries the whole tree, and tar is on any machine that can run
// Ansible — neither scp nor sftp has to be there.
func (c *sshClient) Upload(ctx context.Context, dir string, tarball io.Reader) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("opening a session: %w", err)
	}
	defer session.Close()

	stop := c.closeOnCancel(ctx, session)
	defer stop()

	quoted := shellQuote(dir)
	session.Stdin = tarball
	command := "mkdir -p " + quoted + " && tar -C " + quoted + " -xzf -"
	// tar says what it refused on stderr, and that message is the whole
	// diagnosis when an upload fails.
	out, err := session.CombinedOutput(command)
	if err != nil {
		return fmt.Errorf("sending a directory to %s: %w: %s", dir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// closeOnCancel closes the session when the context is cancelled, and returns
// the function that retires the goroutine once the command is done.
//
// It frees the session, and no more than that: OpenSSH's server ignores the
// signal request, so a cancelled command keeps running there until it ends or
// the connection drops.
func (c *sshClient) closeOnCancel(ctx context.Context, session *ssh.Session) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			session.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

// shellQuote wraps a value so the remote shell takes it as one word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (c *sshClient) Close() error { return c.conn.Close() }
