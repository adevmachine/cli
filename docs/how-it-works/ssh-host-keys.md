# SSH host keys

SSH checks both sides with different keys. Your client key proves who
you are to the server. The server's host key proves which machine
answered you. The host **public** key is safe to store; its private half
never leaves the server.

## First contact is a decision

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

## One store for every connection

Approved keys live in `<config>/known_hosts`, in OpenSSH format, indexed
by machine name rather than address, so a Tailscale address and a public
fallback must present the same identity.

Every command reads that file — `ssh`, `mosh`, `login`, `tunnel`,
`doctor`, `setup`, generated aliases — checking strictly and only
accepting the pinned key. devmachine checks it first with a clear error
on mismatch, then the system SSH process checks again before connecting,
so nothing can swap the key in between.

`known_hosts` holds public keys and is committed by `devmachine setup
git`. Private keys under `keys/` stay ignored and guarded from commits.

## Existing configurations

Configurations from v0.6 and earlier have no pin. Commands do not learn
one silently — they stop and name the fix:

```text
devmachine machines trust <machine>
```

This reads the presented key without authenticating, prints its type and
fingerprint, and asks before writing. `--check` compares without
writing.

## A changed key is not automatically a rotation

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
