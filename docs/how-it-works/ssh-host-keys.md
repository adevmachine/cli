# SSH host keys

SSH checks both sides with different keys. Your client key proves who you
are to the server. The server's host key proves which machine answered
you. The host **public** key is safe to store; its private half never
leaves the server.

## First contact is a decision

Before `setup` offers a client key or password, it reads only the server's
public host key. It prints the key type and SHA256 fingerprint and asks
whether to trust it. Compare that fingerprint with the provider console or
another trusted channel when you need to be sure the address belongs to
the right machine.

Accepting without comparing is trusting on faith that the first server you
reached is the right one. Once you accept it, devmachine remembers that
key, so a different server can never be swapped in later without a
warning. But accepting on faith does not, by itself, prove that first
server was the one you meant to reach. Refusing stops before any login or
remote write.

## One store for every connection

Approved keys live in `<config>/known_hosts`, in OpenSSH format. Each entry
is indexed by the stable machine name, not its IP address, so a Tailscale
address and a public fallback must present the same identity.

Every command reads that same file: `ssh`, `mosh`, `login`, `tunnel`,
`doctor`, `setup`, and aliases generated for ordinary `ssh`. Each one
checks strictly and only accepts the pinned key. devmachine checks the
key itself first, with a clear error if it does not match; then the
system SSH process checks again before it connects, so nothing can swap
the key in between the two checks.

An SSH server can present several valid host keys. Once one is pinned,
scans prefer that key's algorithm rather than mistaking another key from
the same server for a rotation. If the server no longer offers the pinned
algorithm, a scan falls back to its new preferred key, so an explicit
`machines trust --replace` can record an algorithm change.

`known_hosts` holds public keys and is committed by `devmachine setup git`.
Private client keys under `keys/` stay ignored and guarded from commits.

## Existing configurations

Configurations created by v0.6 and earlier have no pin. Normal commands do
not learn one silently; they stop and name the migration command:

```text
devmachine machines trust <machine>
```

The command reads the presented key without authenticating, prints its
type and fingerprint, and asks before writing. `--check` compares without
writing.

## A changed key is not automatically a rotation

A mismatch can mean a deliberate rebuild, the wrong address, or an attack.
The CLI cannot tell those apart, so it prints both fingerprints and
refuses. Verify the new fingerprint outside this SSH connection first. For
a deliberate rebuild, record it explicitly:

```text
devmachine machines trust <machine> --replace
```

Add `--yes` only to skip the local-file confirmation. It does not change
the server and cannot replace a key without `--replace`. `UpdateHostKeys`
is off, so a server cannot rotate this decision on its own.

Removing a machine from `config.yml` leaves its trust entry in place.
Reusing the name must reach the same identity, or go through the same
explicit replacement.
