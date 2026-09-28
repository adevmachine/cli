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

## `tailscale:` entries

An address written `tailscale:<name>` is resolved by asking the
`tailscale` command where that machine is.

When `tailscale` is not installed, or does not know that name, devmachine
**drops that entry and tries the next address** — not an error, since
that is what the other addresses are for. If you do not use Tailscale,
you never have to care this format exists; if you do, your network being
down should not lock you out.

Only running out of addresses is an error, saying what was dropped and
why:

```
no address left to try: tailscale:vps (tailscale is not installed)
```

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
