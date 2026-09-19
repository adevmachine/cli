package keys

import (
	"fmt"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Offered is a key the running SSH agent holds.
//
// A person picks a key by recognising it, and a fingerprint plus the comment is
// what they recognise. PublicKey is the line that goes into authorized_keys: a
// fingerprint names a key but cannot be installed.
type Offered struct {
	Fingerprint string
	Comment     string
	PublicKey   string
}

// FromAgent lists what the running SSH agent holds.
//
// This is how a key kept in a password manager is supported without the CLI
// knowing anything about that manager: it asks the agent what it has. Somebody
// whose key never touches the disk has no file to point at.
//
// No agent is not an error, and neither is a socket nothing answers on: both
// are ordinary, and neither is a reason to stop what the caller is doing.
func FromAgent() ([]Offered, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, nil
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, nil
	}
	defer func() { _ = conn.Close() }()

	held, err := agent.NewClient(conn).List()
	if err != nil {
		// Something is listening and will not talk. Saying so is better than
		// reporting an empty agent, which sends the person looking in the
		// wrong place.
		return nil, fmt.Errorf("asking the SSH agent at %s what it holds: %w", sock, err)
	}

	offered := make([]Offered, 0, len(held))
	for _, key := range held {
		offered = append(offered, Offered{
			Fingerprint: ssh.FingerprintSHA256(key),
			Comment:     key.Comment,
			PublicKey:   strings.TrimSpace(key.String()),
		})
	}
	return offered, nil
}
