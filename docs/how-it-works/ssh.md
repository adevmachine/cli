# SSH: logging in and knowing it is your server

SSH checks both sides with different keys. Your key proves who you are to
the server. The server's host key proves which machine answered you.

## Your key: how devmachine logs in

If a machine has a `key:` in your configuration, devmachine uses that key,
and only that key. The key `setup` makes for you is always used this way.

The reason: a server lets you try only a few times — six, usually — and
then hangs up. Offering every key you have could use up those tries on the
wrong keys before reaching the right one, and you would get a refusal that
makes no sense.

When you pick a key from the SSH agent instead — in `setup` or `machines
add`, the choice that ends in "(from the SSH agent)" — devmachine records
its public half as `agent_key:` in `config.yml`. From then on it asks the
agent for that one key, the same way `key:` asks a file for its one key.
The private half never leaves the agent; only the public key, which is not
a secret, is written down.

A machine with neither `key:` nor `agent_key:` — set up before this
existed, or edited by hand — falls back to asking the agent for
everything it holds, which is the risk a many-key agent (a password
manager's, say) can run into. Add `agent_key:` to it by hand, set to the
line `ssh-add -L` prints for the key you want. See [logging in with your
1Password SSH key](../guides/log-in-with-1password.md).

If the agent no longer holds the recorded key — locked, or a different
`SSH_AUTH_SOCK` — devmachine says so by fingerprint rather than trying
every key it can find instead.

### Two different programs

| Job | What runs | Why |
| --- | --- | --- |
| commands, checks, stats | the Go SSH library | the CLI controls authentication and reads the output |
| `ssh`, `mosh` | the system binary | you want your terminal, your agent, your tmux — not an imitation |

`devmachine ssh` hands the terminal over: it does nothing differently
from running `ssh` yourself.

### Which account

A machine's `user:` is its **admin account** — usually `root` — used for
checking and setting the server up. A workspace has a separate account,
and `devmachine ssh <workspace>` logs into that; with no workspace named,
it logs in as the admin.

## The server's key: how devmachine knows it is your server

The host **public** key is safe to store; its private half never leaves
the server.

### First contact is a decision

Before `setup` offers a client key or password, it reads only the
server's public host key, prints its type and SHA256 fingerprint, and
asks whether to trust it. Compare that fingerprint with the provider
console when you need to be sure the address belongs to the right
machine.

Accepting without comparing is trusting on faith that the first server
you reached is the right one. Once accepted, devmachine remembers that
key, so a different server can never swap in later without a warning —
but faith alone does not prove the first server was the right one.
Refusing stops before any login or remote write.

### One store for every connection

Approved keys live in `<config>/known_hosts`, in OpenSSH format, indexed
by machine name rather than address, so a Tailscale address and a public
fallback must present the same identity.

Every command reads that file — `ssh`, `mosh`, `login`, `tunnel`,
`doctor`, `setup`, generated aliases — checking strictly and only
accepting the pinned key. Once a key is stored, every command requires
that exact identity before it offers authentication — a missing or
different key fails closed instead of being learned silently. devmachine
checks it first with a clear error on mismatch, then the system SSH
process checks again before connecting, so nothing can swap the key in
between.

`known_hosts` holds public keys and is committed by `devmachine setup
git`. Private keys under `keys/` stay ignored and guarded from commits.

### Existing configurations

Configurations from v0.6 and earlier have no pin. Commands do not learn
one silently — they stop and name the fix:

```text
devmachine machines trust <machine>
```

This reads the presented key without authenticating, prints its type and
fingerprint, and asks before writing. `--check` compares without
writing.

### A changed key is not automatically a rotation

A mismatch can mean a deliberate rebuild, the wrong address, or an
attack. devmachine cannot tell those apart, so it prints both
fingerprints and refuses. Verify the new fingerprint outside this SSH
connection first, then record a deliberate rebuild explicitly:

```text
devmachine machines trust <machine> --replace
```

`--yes` only skips the local confirmation — it cannot replace a key
without `--replace`, and never touches the server. `UpdateHostKeys` is
off, so a server can never rotate this decision on its own.

Removing a machine from `config.yml` leaves its trust entry in place.
Reusing the name must reach the same identity, or go through the same
explicit replacement.
