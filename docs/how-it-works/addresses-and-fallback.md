# Addresses and fallback

A machine can answer on more than one address:

```yaml
machines:
  - name: main
    hosts:
      - tailscale:vps
      - 203.0.113.10
```

devmachine tries each in order and uses the first one that answers,
printing which on stderr. A public address can stop working for you
while it keeps working for everyone else — a provider's network can drop
your traffic, your own can block a port. A second address costs one line
and turns an outage into a slower connection.

## `<prefix>:<name>` entries

An address written `<prefix>:<name>`, such as `tailscale:vps`, is not an
address yet. It belongs to a private network, and the **network package**
that declares that prefix turns it into one. The CLI knows the prefix and
where the package's scripts are. It does not know Tailscale, Headscale or
any other product: each is a package, and a new one needs no new CLI.

To resolve `tailscale:vps`, devmachine:

1. Finds the package whose `package.yml` declares `network.prefix:
   tailscale`. A package this machine installs wins; otherwise any package
   in the pinned release or in your own `packages/` folder will do. Only a
   release a sync already downloaded counts — a connection never waits on
   a download.
2. Runs that package's `resolve` script **on your computer**, with `vps` as
   its one argument and at most 5 seconds to answer.
3. Tries the addresses it printed, in its order, then the next entry in
   `hosts`.

When the script exits with status 3 — "this network is not reachable from
here": the app is missing, off, or signed out — devmachine **drops that
entry and tries the next address**. Not an error, since that is what the
other addresses are for. If you do not use the network, you never have to
care; if you do, your network being down should not lock you out.

A script that fails in any other way, prints something that is not an IP
address, or takes too long is dropped the same way, with its own reason.
It is a bug to fix, but the public address still works, and refusing to try
it would turn a broken script into an outage.

An entry whose prefix no package declares is dropped too, and says so. An
IPv6 address such as `2001:db8::1` is never read as a prefix.

Only running out of addresses is an error, saying what was dropped and
why:

```
no address left to try: tailscale:vps (tailscale is not running)
```

`devmachine resolve` prints the same list without connecting: every address
in the order it is tried, which entry and package it came from, and every
entry that was dropped. It is what the macOS app calls, and what to run when
a connection goes somewhere you did not expect.

### The built-in `tailscale:` resolver (deprecated)

Before network packages, the CLI resolved `tailscale:` itself, by running
`tailscale status --json`. It still does, but only when no package declares
the `tailscale` prefix — that is, with a packages release from before the
`network:` block. It will go away once no supported release needs it. Pin a
newer release and nothing changes for you: the `tailscale` package asks the
same command.

## SSH aliases that resolve when you connect

The aliases `devmachine aliases --write` puts in `~/.ssh/config` do not hold
an address. Each one says:

```
Host acme-devmachine
    HostName main
    ProxyCommand /opt/homebrew/bin/devmachine --config /Users/you/.config/devmachine ssh-proxy main %p
    HostKeyAlias main-devmachine
    ...
```

When ssh connects, `devmachine ssh-proxy` resolves the machine's addresses
as above, opens a TCP connection to the first that answers, and passes
bytes between it and ssh. ssh does everything else as before — the key, the
pinned host key, the session.

Why not write the address itself, as older versions did? An address in a
file is only right until the network changes. Turn Tailscale off, and an
alias holding the tailnet address fails until you run `aliases --write`
again. Resolving at connect time means the file only changes when a
workspace or machine does.

The trade-offs:

- **The CLI has to be where the alias says.** The alias names the
  `devmachine` found on your `PATH`, by its full path, so an editor or app
  with a different `PATH` still finds it. When `devmachine` is not on your
  `PATH`, `aliases` writes the old form with the address resolved now.
  `devmachine doctor` warns when the file does not match what `aliases`
  would write today.
- **Only the connection falls back.** Once an address answers, the SSH
  handshake has started on it, and cannot move to another. An address that
  answers and then refuses the login is a login problem, not a network one.
- **The host key is still checked.** `HostKeyAlias` and
  `UserKnownHostsFile` make ssh check the key pinned for the machine,
  whichever address it came through.
- **mosh.** `mosh` starts over ssh, so the alias works for that part. But
  its own UDP connection needs an IP address, and by default mosh finds one
  with its own ProxyCommand, which replaces this one. Use
  `mosh --experimental-remote-ip=remote acme-devmachine`, where the server
  says which address ssh reached, or `devmachine mosh acme`. A program
  that needs the address itself asks `devmachine resolve`.
- **`-pub`.** When a machine has a private entry and a public one, an
  `acme-devmachine-pub` alias still goes straight to the public address, for
  when you want to skip the private network on purpose.

## Answered, or refused?

These are different problems, and devmachine tells them apart:

```
machine "main": no address answered: 203.0.113.10 (dial tcp: i/o timeout)
```

Nothing is listening, or nothing can reach it — check the network, the
port, the firewall.

```
machine "main" answered on 203.0.113.10 but refused the login for "bob": ...
```

The machine is there; the account or key is the problem. Reporting both
as "nothing answered" would send you to check a network that was never
at fault.
