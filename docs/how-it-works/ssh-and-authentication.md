# SSH and authentication

## The CLI offers one method, never everything it has

When a machine has `key:`, the CLI offers that key and nothing else.
Otherwise it offers the SSH agent and nothing else. Never both.

This is not tidiness. **An SSH server gives up after a few attempts** —
`MaxAuthTries`, six by default. A client that offers every key an agent
holds burns those attempts on keys the server does not want, cutting the
connection before the method that would have worked is ever tried.

The failure is confusing: everything works, then you add two unrelated
keys to your agent, and hosts that worked yesterday start refusing you
with an error that names no key. It is worse for a password — a server
that accepts one will never ask for it if the agent used up the attempts
first, so "it never prompts me" looks like the server refusing
passwords.

The CLI talks to the SSH library directly, instead of running the `ssh`
command, so it can decide exactly what to offer.

## Two different programs

| Job | What runs | Why |
| --- | --- | --- |
| commands, checks, stats | the Go SSH library | the CLI controls authentication and reads the output |
| `ssh`, `mosh` | the system binary | you want your terminal, your agent, your tmux — not an imitation |

`devmachine ssh` hands the terminal over: it does nothing differently
from running `ssh` yourself.

## Which account

A machine's `user:` is its **admin account** — usually `root` — used for
checking and setting the server up. A workspace has a separate account,
and `devmachine ssh <workspace>` logs into that; with no workspace named,
it logs in as the admin.

## Host keys

Your key proves who you are to the server. The server's host key proves
which server answered you — different keys.

During `setup`, the CLI reads the host's public key first, prints its
SHA256 fingerprint, and asks before trusting it. Compare that fingerprint
with the provider console: accepting without comparing pins the first
server reached, but does not on its own prove it was the right one.

Approved host keys live in `<config>/known_hosts`, indexed by machine
name. Once a key is stored, every command requires that exact identity
before it offers authentication — a missing or different key fails
closed instead of being learned silently.

`ssh`, `mosh`, `login`, `tunnel` and generated aliases all use the same
store and the same strict check. See [SSH host
keys](ssh-host-keys.md) for migration and mismatches.
