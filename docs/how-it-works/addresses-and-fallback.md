# Addresses and fallback

A machine can answer on more than one address:

```yaml
machines:
  - name: main
    hosts:
      - tailscale:vps
      - 203.0.113.10
```

They are tried in order. The first that answers is used, and the CLI says on
stderr which one that was.

## Why more than one

A public address can stop working for you while working for everybody else. A
provider's edge can drop your traffic, a network can block a port, a route can
break in one direction. When that happens the machine is fine and unreachable at
the same time.

A second path costs one line and turns an outage into a slower connection.

## tailscale: entries

An address written `tailscale:<name>` is resolved by asking the `tailscale`
command where that machine is.

When `tailscale` is not installed, or does not know that name, **the entry is
dropped and the next address is tried**. It is not an error: the entries after
it are the fallbacks it exists for.

This is deliberate. Someone who does not use Tailscale should never have to care
that the format exists, and someone who does should not lose access when their
network is down.

Only an empty result is an error, and it says what was dropped and why:

```
no address left to try: tailscale:vps (tailscale is not installed)
```

## Answered, or refused?

These are different problems and the CLI says which happened:

```
machine "main": no address answered: 203.0.113.10 (dial tcp: i/o timeout)
```

Nothing is listening, or nothing can reach it. Check the network, the port, the
firewall.

```
machine "main" answered on 203.0.113.10 but refused the login for "bob": ...
```

The machine is there. The account or the key is the problem.

Reporting both as "nothing answered" would send people to inspect a network that
was never at fault.
