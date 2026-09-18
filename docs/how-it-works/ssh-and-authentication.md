# SSH and authentication

## The CLI offers one method, never everything it has

When a machine has `key:`, the CLI offers that key and nothing else. When it
does not, the CLI offers the SSH agent and nothing else. It never offers both.

This is not tidiness. **An SSH server gives up after a few attempts** —
`MaxAuthTries`, six by default. A client that offers every key an agent holds
burns those attempts on keys the server does not want, and the connection is cut
before the method that would have worked is ever tried.

The failure is bewildering when it happens. Everything works; you add two keys
to your agent for something unrelated; hosts that worked yesterday start
refusing you, with an error that names no key and suggests no cause.

It is worse for a password. A server that accepts a password will never ask for
one if the agent used up the attempts first — so "it never prompts me" looks
like the server refusing passwords, when the client never got that far.

Using the SSH library directly, rather than running the `ssh` command, is what
makes this controllable: the CLI decides what to offer.

## Two different programs

| Job | What runs | Why |
| --- | --- | --- |
| commands, checks, stats | the Go SSH library | the CLI controls the authentication and reads the output |
| `ssh`, `mosh` | the system binary | you want your terminal, your agent, your tmux — not an imitation |

`devmachine ssh` hands the terminal over. Anything it did differently from
running `ssh` yourself would be a surprise, so it does not try.

## Which account

A machine's `user:` is its **administrative** login, used for checking and
provisioning — usually `root`.

A workspace's account is separate, and that is what `devmachine ssh <workspace>`
logs in as. `devmachine ssh` with no workspace logs in as the machine's admin.

## Host keys

The CLI does not verify the machine's host key today. It reaches a server you
already own, over a path you chose. Pinning would need a store of known keys
that does not exist yet; it belongs with the first-contact flow, which is where
trust is actually established.

This is worth knowing if you are on a network you do not trust.
