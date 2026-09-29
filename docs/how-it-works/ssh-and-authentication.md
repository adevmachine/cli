# SSH and authentication

## devmachine logs in with one key, not all of yours

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
1Password SSH key](../examples/log-in-with-1password.md).

If the agent no longer holds the recorded key — locked, or a different
`SSH_AUTH_SOCK` — devmachine says so by fingerprint rather than trying
every key it can find instead.

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
