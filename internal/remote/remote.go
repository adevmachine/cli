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
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/hostkeys"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SelfAddress is what Dial reports for a self machine: there is no address,
// only this computer.
const SelfAddress = "this computer"

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
	realDialContext = dialContext
	dialSSH         = realDialContext
)

// Client runs commands on the machine.
type Client interface {
	Run(ctx context.Context, command string) (string, error)
	// RunInput runs a command with stdin fed from the reader.
	//
	// It is how a value reaches the machine without going in an argument,
	// where `ps` shows it to every account on the machine.
	RunInput(ctx context.Context, command string, stdin io.Reader) (string, error)
	// Stream runs a command with its output going to the writers as it
	// arrives, and returns the exit status.
	Stream(ctx context.Context, command string, stdout, stderr io.Writer) error
	// Upload extracts a gzipped tar into a directory on the machine.
	Upload(ctx context.Context, dir string, tarball io.Reader) error
	Close() error
}

// Literal reports whether a host entry is an address ssh can dial as written,
// rather than a name something closer to the network has to resolve.
func Literal(address string) bool {
	return !strings.HasPrefix(address, tailscalePrefix)
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

// ErrAuthRefused says the machine answered and then refused the login.
//
// It is a different problem from nothing answering, with a different fix, so a
// caller can tell them apart without matching strings.
var ErrAuthRefused = errors.New("the machine refused the login")

// ErrHostKeyUnknown says no identity has been trusted for the machine.
var ErrHostKeyUnknown = errors.New("the SSH host key is not trusted")

// ErrHostKeyChanged says the machine presented a key different from its pin.
var ErrHostKeyChanged = errors.New("the SSH host key changed")

// Auth says how to authenticate. Exactly one field is set.
//
// Never more than one: a server counts every method offered against
// MaxAuthTries and cuts the connection when the count runs out, so a list ends
// the session before the method that would have worked is reached.
type Auth struct {
	KeyPath  string
	Password string
	Agent    bool
}

// describe names the method in the words of whoever has to fix it.
func (a Auth) describe() string {
	switch {
	case a.KeyPath != "":
		return "the key " + a.KeyPath
	case a.Password != "":
		return "a password"
	case a.Agent:
		return "a key from the SSH agent"
	}
	return "nothing"
}

// Dial tries each of a machine's addresses in order and returns the first
// client that answered, along with the address it answered on.
//
// user is who to log in as. Empty means the machine's administrative login;
// a workspace passes its own Linux account instead.
func Dial(ctx context.Context, m config.Machine, user string) (Client, string, error) {
	if m.Self {
		return &localClient{}, SelfAddress, nil
	}
	callback, algorithms, err := hostKeyPolicy(m)
	if err != nil {
		return nil, "", err
	}
	a, err := authFor(m)
	if err != nil {
		return nil, "", err
	}
	return dialWithPolicy(ctx, m, user, a, callback, algorithms)
}

// authFor is what a machine's configuration implies: its own key when it has
// one, and the agent otherwise.
func authFor(m config.Machine) (Auth, error) {
	if m.Key != "" {
		return Auth{KeyPath: m.Key}, nil
	}
	if os.Getenv("SSH_AUTH_SOCK") == "" {
		return Auth{}, fmt.Errorf("no key in the configuration and no SSH agent: set `key` in config.yml or start an agent")
	}
	return Auth{Agent: true}, nil
}

// DialWith is Dial with the authentication method chosen by the caller, which
// is what the bootstrap needs: a password before any key exists, then the key
// alone to prove it.
func DialWith(ctx context.Context, m config.Machine, user string, a Auth) (Client, string, error) {
	callback, algorithms, err := hostKeyPolicy(m)
	if err != nil {
		return nil, "", err
	}
	return dialWithPolicy(ctx, m, user, a, callback, algorithms)
}

func dialWithPolicy(ctx context.Context, m config.Machine, user string, a Auth, callback ssh.HostKeyCallback, algorithms []string) (Client, string, error) {
	auth, err := authMethods(a)
	if err != nil {
		return nil, "", err
	}
	addresses, err := Resolve(m)
	if err != nil {
		return nil, "", err
	}
	if user == "" {
		user = m.User
	}

	cfg := &ssh.ClientConfig{
		User:              user,
		Auth:              auth,
		HostKeyCallback:   callback,
		HostKeyAlgorithms: algorithms,
		Timeout:           dialTimeout,
	}

	var (
		failures []string
		authErr  error
	)
	for _, address := range addresses {
		target := net.JoinHostPort(address, fmt.Sprint(m.Port))
		conn, err := dialSSH(ctx, target, cfg)
		if err == nil {
			return &sshClient{conn: conn}, address, nil
		}
		if errors.Is(err, ErrHostKeyUnknown) || errors.Is(err, ErrHostKeyChanged) {
			return nil, "", err
		}
		// A machine that answers and then refuses the login is a different
		// problem from one that never answered, and the fix is different too.
		// Reporting both as "nothing answered" sends people to check the
		// network when the account or the key is what is wrong.
		if isAuthFailure(err) && authErr == nil {
			authErr = fmt.Errorf("%w: machine %q answered on %s but refused %s for %q: %w",
				ErrAuthRefused, m.Name, address, a.describe(), user, err)
		}
		failures = append(failures, fmt.Sprintf("%s (%v)", address, err))
	}
	if authErr != nil {
		return nil, "", authErr
	}
	return nil, "", fmt.Errorf("machine %q: no address answered: %s", m.Name, strings.Join(failures, "; "))
}

func hostKeyPolicy(m config.Machine) (ssh.HostKeyCallback, []string, error) {
	trustCommand := fmt.Sprintf("devmachine machines trust %s", m.Name)
	if m.KnownHostsFile == "" {
		return nil, nil, fmt.Errorf("%w for machine %q: run `%s`", ErrHostKeyUnknown, m.Name, trustCommand)
	}
	store, err := hostkeys.Open(m.KnownHostsFile)
	if err != nil {
		return nil, nil, err
	}
	pinned, err := store.Key(m.Name, m.Port)
	if err != nil {
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
			return nil, nil, fmt.Errorf("%w for machine %q: run `%s`", ErrHostKeyUnknown, m.Name, trustCommand)
		}
		return nil, nil, err
	}
	callback, err := knownhosts.New(m.KnownHostsFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading SSH host trust from %s: %w", m.KnownHostsFile, err)
	}
	wrapped := func(hostname string, remote net.Addr, presented ssh.PublicKey) error {
		aliasAddress := net.JoinHostPort(hostkeys.Alias(m.Name), strconv.Itoa(m.Port))
		if err := callback(aliasAddress, remote, presented); err != nil {
			var keyErr *knownhosts.KeyError
			if errors.As(err, &keyErr) {
				if len(keyErr.Want) == 0 {
					return fmt.Errorf("%w for machine %q: run `%s`", ErrHostKeyUnknown, m.Name, trustCommand)
				}
				address := hostname
				if host, _, splitErr := net.SplitHostPort(hostname); splitErr == nil {
					address = host
				}
				return fmt.Errorf("%w for machine %q at %s: expected %s, received %s; verify the address, then run `%s --replace` only for a deliberate rebuild",
					ErrHostKeyChanged, m.Name, address, hostkeys.Fingerprint(pinned), hostkeys.Fingerprint(presented), trustCommand)
			}
			return err
		}
		return nil
	}
	return wrapped, hostkeys.Algorithms(pinned), nil
}

// ScanHostKey reads a server's public host key without offering any
// authentication method.
func ScanHostKey(ctx context.Context, m config.Machine) (ssh.PublicKey, string, error) {
	addresses, err := Resolve(m)
	if err != nil {
		return nil, "", err
	}
	algorithms, err := scanHostKeyAlgorithms(m)
	if err != nil {
		return nil, "", err
	}
	user := m.User
	if user == "" {
		user = config.DefaultAdminUser
	}
	var failures []string
	for _, address := range addresses {
		target := net.JoinHostPort(address, strconv.Itoa(m.Port))
		presented, answered, handshakeErr := scanPresentedHostKey(ctx, target, user, algorithms)
		if !answered {
			failures = append(failures, fmt.Sprintf("%s (%v)", address, handshakeErr))
			continue
		}
		// A deliberate rebuild can replace one host-key algorithm with another.
		// Prefer the pinned algorithm when the server still offers it, but fall
		// back to ordinary negotiation so `machines trust --replace` can inspect
		// the new identity when it does not.
		if presented == nil && len(algorithms) > 0 {
			presented, answered, handshakeErr = scanPresentedHostKey(ctx, target, user, nil)
		}
		if !answered {
			failures = append(failures, fmt.Sprintf("%s (%v)", address, handshakeErr))
			continue
		}
		if presented != nil {
			return presented, address, nil
		}
		return nil, "", fmt.Errorf("machine %q answered on %s but did not present an SSH host key: %w", m.Name, address, handshakeErr)
	}
	return nil, "", fmt.Errorf("machine %q: no address answered while scanning its SSH host key: %s", m.Name, strings.Join(failures, "; "))
}

func scanHostKeyAlgorithms(m config.Machine) ([]string, error) {
	if m.KnownHostsFile == "" {
		return nil, nil
	}
	store, err := hostkeys.Open(m.KnownHostsFile)
	if err != nil {
		return nil, err
	}
	pinned, err := store.Key(m.Name, m.Port)
	if err != nil {
		var unknown *knownhosts.KeyError
		if errors.As(err, &unknown) && len(unknown.Want) == 0 {
			return nil, nil
		}
		return nil, err
	}
	return hostkeys.Algorithms(pinned), nil
}

func scanPresentedHostKey(ctx context.Context, target, user string, algorithms []string) (ssh.PublicKey, bool, error) {
	conn, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, false, err
	}
	var presented ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User: user,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			presented = key
			return nil
		},
		HostKeyAlgorithms: algorithms,
		Timeout:           dialTimeout,
	}
	clientConn, _, _, handshakeErr := ssh.NewClientConn(conn, target, cfg)
	if clientConn != nil {
		_ = clientConn.Close()
	} else {
		_ = conn.Close()
	}
	return presented, true, handshakeErr
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

// authMethods turns an Auth into the one method it names.
//
// More than one is refused rather than merged: offering everything is what
// trips MaxAuthTries.
func authMethods(a Auth) ([]ssh.AuthMethod, error) {
	var chosen []string
	if a.KeyPath != "" {
		chosen = append(chosen, "a key")
	}
	if a.Password != "" {
		chosen = append(chosen, "a password")
	}
	if a.Agent {
		chosen = append(chosen, "the SSH agent")
	}
	if len(chosen) != 1 {
		return nil, fmt.Errorf("exactly one authentication method can be offered, got %d (%s)",
			len(chosen), strings.Join(chosen, ", "))
	}

	switch {
	case a.KeyPath != "":
		body, err := os.ReadFile(a.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("reading the private key %s: %w", a.KeyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(body)
		if err != nil {
			return nil, fmt.Errorf("parsing the private key %s: %w", a.KeyPath, err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil

	case a.Password != "":
		return []ssh.AuthMethod{ssh.Password(a.Password)}, nil
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

// RunInput executes one command with stdin fed from the reader.
func (c *sshClient) RunInput(ctx context.Context, command string, stdin io.Reader) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("opening a session: %w", err)
	}
	defer session.Close()

	stop := c.closeOnCancel(ctx, session)
	defer stop()

	session.Stdin = stdin
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

// localClient runs commands on this computer instead of over SSH, for a
// machine that declares `self: true`. Every command still goes through
// /bin/bash -c, exactly as it does on a remote machine, so a package's tasks
// see the same shell either way.
type localClient struct{}

// Run executes one command and returns its standard output, matching
// sshClient.Run: stderr is not collected, only what the command wrote to
// stdout.
func (c *localClient) Run(ctx context.Context, command string) (string, error) {
	out, err := exec.CommandContext(ctx, "/bin/bash", "-c", command).Output()
	if err != nil {
		return string(out), fmt.Errorf("running %q: %w", command, err)
	}
	return string(out), nil
}

// RunInput executes one command with stdin fed from the reader.
func (c *localClient) RunInput(ctx context.Context, command string, stdin io.Reader) (string, error) {
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", command)
	cmd.Stdin = stdin
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("running %q: %w", command, err)
	}
	return string(out), nil
}

// Stream executes one command with its output reaching the writers as it
// arrives.
func (c *localClient) Stream(ctx context.Context, command string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", command)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running %q: %w", command, err)
	}
	return nil
}

// Upload extracts a gzipped tar into a directory on this computer.
func (c *localClient) Upload(_ context.Context, dir string, tarball io.Reader) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := extractTarGz(dir, tarball); err != nil {
		return fmt.Errorf("sending a directory to %s: %w", dir, err)
	}
	return nil
}

func (c *localClient) Close() error { return nil }

// extractTarGz writes a gzipped tar's regular files and directories under
// dir, refusing any entry that would land outside it.
func extractTarGz(dir string, r io.Reader) error {
	zipped, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("reading the archive: %w", err)
	}
	defer func() { _ = zipped.Close() }()

	archive := tar.NewReader(zipped)
	root := filepath.Clean(dir)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading the archive: %w", err)
		}

		target := filepath.Join(root, filepath.FromSlash(header.Name))
		if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("%s escapes %s: refusing to extract it", header.Name, dir)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, header.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, archive); err != nil {
				file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
	}
}
