# SSH host keys

SSH authenticates both sides with different keys. Your client key proves who
you are to the server. The server's host key proves which machine answered
you. The host **public** key is safe to store; its private half never leaves the
server.

## First contact is a decision

Before `setup` offers a client key or password, it reads only the server's
public host key. It prints the key type and SHA256 fingerprint and asks whether
to trust it. Compare that fingerprint with the provider console or another
trusted channel when you need assurance that the address belongs to the right
machine.

Accepting without comparing is trust on first use: it pins the first server
reached. It prevents a different server being accepted later, but it does not
independently prove the first server was the intended one. Refusing stops
before authentication or a remote write.

## One store for every connection

Approved keys live in `<config>/known_hosts`, in OpenSSH format. The entry is
indexed by the stable machine name, not its IP address, so a Tailscale address
and a public fallback must present the same identity.

Every path reads that same file: programmatic commands, `ssh`, `mosh`, `login`,
`tunnel`, `doctor`, setup, and aliases generated for ordinary `ssh`. They use
strict checking and restrict negotiation to the pinned key's compatible
algorithms. A preflight scan gives CLI diagnostics before a system SSH process;
the process verifies again, closing a change-between-check-and-use race.

An SSH server can present several valid host keys. Once one is pinned, scans
prefer that key's algorithm rather than mistaking another key from the same
server for a rotation. If the server no longer offers the pinned algorithm, a
scan falls back to its new preferred key so an explicitly verified
`machines trust --replace` can record an algorithm change.

`known_hosts` contains public keys and is committed by `devmachine setup git`.
Private client keys under `keys/` remain ignored and guarded from commits.

## Existing configurations

Configurations created by v0.6 and earlier have no pin. Normal commands do not
learn one silently; they stop and name the migration command:

```text
devmachine machines trust <machine>
```

The command reads the presented key without authenticating, prints its type and
fingerprint, and asks before writing. `--check` compares without writing.

## A changed key is not automatically a rotation

A mismatch can mean a deliberate rebuild, the wrong address, or an attack.
The CLI cannot distinguish those cases, so it prints both fingerprints and
refuses. Verify the new fingerprint outside this SSH connection first. For a
deliberate rebuild, record it explicitly:

```text
devmachine machines trust <machine> --replace
```

Add `--yes` only to skip the local-file confirmation. It does not change the
server and cannot replace a key without `--replace`. `UpdateHostKeys` is
disabled, so a server cannot rotate this decision on its own.

Removing a machine from `config.yml` leaves its trust entry intact. Reusing
the name must reach the same identity or go through the same explicit
replacement.
