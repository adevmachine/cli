# DNS

`devmachine dns` points a domain name at your server, so people can reach
it by name instead of a bare IP address, and checks that it worked.

## Zone and record

A **zone** is a domain: `example.com`. A **record** is one name, one
type, and one value: `www.example.com A 198.51.100.10`.

`dns add` makes a name hold **exactly** the one value you give it — it
replaces whatever was there, never adds to it. Most names need only one
address, and this keeps every provider honest about what "add" means:
silently appending is how a name ends up with two answers, one of them
stale. A name that needs several values on purpose is out of scope for
`dns add` — use `dns list` to see what's there, and your provider's own
tools for that case.

## `@` is the apex

The **apex** is the bare root of your domain — `example.com`, as opposed
to `www.example.com` or `app.example.com`. It has no label of its own, so
write `@` for it, the way most registrars do:

```
devmachine dns add example.com A 198.51.100.10
devmachine dns add @ A 198.51.100.10 --zone example.com
```

`www.example.com` is the label `www`; `example.com` is the label `@`. The
CLI works this out on its own, so most of the time you just type the full
name.

## Supported types, and why two are refused

`A`, `AAAA`, `CNAME`, `TXT`. Not `MX`, not `SRV` — those carry more than a
value (a priority, sometimes a weight and a port), and registrars store
that extra data differently. A single value can't cover both, so the CLI
refuses these two by name instead of guessing a shape that only works on
some providers.

## TTL zero means "choose"

A record with no TTL — how long it should be cached — asks the provider
to pick its own default, instead of the CLI inventing a number that means
nothing to the registrar. `dns add` never asks for one.

## A provider is a package

Nothing that can write to a registrar ships inside the CLI. Talking to
Hostinger, Cloudflare, or any other registrar is a package like any other
— installed with `devmachine packages add`, applied with `devmachine
sync`, and run on the machine, never on your computer.

Until a provider package is installed on the machine, the CLI can't write
anywhere. See
[the DNS provider contract](../reference/dns-provider-contract.md) for
what a provider package is, and
[DNS providers](../how-it-works/dns-providers.md) for what each one does.

## Choosing a zone and a provider

A command that acts on a name works out two things: which zone it falls
in, and which installed provider holds that zone.

1. **`--dns-provider`** names one directly — no guessing.
2. Otherwise, the CLI asks every installed provider which zones it can
   see, and picks the one whose zone the name best matches.
3. If none claims the zone — none is installed, or the machine can't be
   reached — the answer is **manual**: the CLI prints the record for you
   to create by hand.
4. If two providers claim the same zone, the CLI stops and asks you to
   pick with `--dns-provider`.

For example, with `hostinger` holding `example.com` and `cloudflare`
holding `example.net`, both installed:

```
devmachine dns add www.example.com A 198.51.100.10
```

resolves to `hostinger`. Every command prints which provider it picked
and why, so a surprising answer is never silent.

The command asks right before it writes. For scripts, `--publish` says
public DNS is intended on purpose; `--yes` alone never creates a record.

## `dns status` is not `dns list`

`dns list` and `dns check` ask the **registrar**: what's configured
there, whether or not it has spread across the internet yet. `dns status`
instead asks **the internet**: it resolves the name and fetches its
certificate from your computer, the way anybody else would see it. It
needs no provider and no machine.

A record can be right in `dns list` and still fail `dns status`, because
it hasn't spread yet. It can be right in `dns status` for a name no
installed provider holds, because somebody set it by hand. The two
answer different questions on purpose.
