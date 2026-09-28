# Addresses and fallback

A machine can answer on more than one address:

```yaml
machines:
  - name: main
    hosts:
      - tailscale:vps
      - 203.0.113.10
```

devmachine tries each address in order and uses the first one that answers.
It prints which one that was on stderr.

## Why more than one

A public address can stop working for you while it keeps working for
everyone else. A provider's network can drop your traffic, your own
network can block a port, a route can break in one direction. When that
happens, the server is fine — it is just unreachable from where you are.

A second address costs one line in the config. It turns an outage into a
slower connection instead of no connection.

## `tailscale:` entries

An address written `tailscale:<name>` is resolved by asking the
`tailscale` command where that machine is.

When `tailscale` is not installed, or does not know that name, devmachine
**drops that entry and tries the next address**. This is not an error —
the other addresses are the fallback the format exists for.

That is on purpose. If you do not use Tailscale, you should never have to
care that this format exists. If you do, your network being down should
not lock you out.

Only running out of addresses is an error, and it says what was dropped
and why:

```
no address left to try: tailscale:vps (tailscale is not installed)
```

## Answered, or refused?

These are different problems, and devmachine tells them apart:

```
machine "main": no address answered: 203.0.113.10 (dial tcp: i/o timeout)
```

Nothing is listening, or nothing can reach it. Check the network, the
port, the firewall.

```
machine "main" answered on 203.0.113.10 but refused the login for "bob": ...
```

The machine is there. The account or the key is the problem.

Reporting both as "nothing answered" would send you off to check a
network that was never at fault.
