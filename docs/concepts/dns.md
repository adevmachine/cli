# DNS

`devmachine dns` points a name at this machine, and checks that it took effect.

## Zone and record

A **zone** is a domain: `example.com`. A **record** is one name, one type, and
one value: `www.example.com A 198.51.100.10`.

The CLI's record is one value, never a set. `dns add` makes a name hold
**exactly** that one value — it replaces whatever was there, it does not add to
it. Most names need one address, and asking for one value keeps every provider
honest about what "add" does: appending silently is how a name ends up serving
two answers, one of which is stale.

A name that must hold several values on purpose — round-robin, multiple TXT
records — is out of scope for `dns add`. Use `dns list` to see what is there,
and a provider's own tools for that case.

## `@` is the apex

The zone itself has no label. Write `@` for it, the way most registrars do:

```
devmachine dns add example.com A 198.51.100.10
devmachine dns add @ A 198.51.100.10 --zone example.com
```

`www.example.com` is the label `www`; `example.com` is the label `@`. The CLI
works this out from the zone it resolved, so most of the time you type the full
name and never think about the label at all.

## Supported types, and why two are refused

`A`, `AAAA`, `CNAME`, `TXT`. Not `MX`, not `SRV`.

Both carry more than a value: a priority, sometimes a weight and a port. One
registrar folds that into the same string as the address; another keeps it in
its own field. A single `value` string cannot round-trip on both, so the CLI
refuses them by name instead of guessing a shape that only works on some
providers.

## TTL zero means "choose"

A record with no TTL asks the provider to pick its own default, rather than the
CLI inventing a number that means nothing to the registrar. `dns add` never
asks for a TTL; a provider that wants one picks it on its own.

## A provider is a package

Nothing that can write to a registrar ships inside the CLI. Talking to
Hostinger, Cloudflare, or any other registrar is a package like any other —
installed with `devmachine packages add`, converged with `devmachine sync`,
and run on the machine, never on your computer.

"Installed" means the lock file says the package is on this machine and its
kind is `dns`. Until then, the CLI cannot write anywhere: see
[the DNS provider contract](../reference/dns-provider-contract.md) for what a
provider package is and how it is called, and
[DNS providers](../how-it-works/dns-providers.md) for what each shipped
provider actually does and the mistakes it avoids.

## Choosing a zone and a provider

A command that acts on a name — `dns list`, `dns check`, `dns add`, `dns rm` —
has to work out two things: which zone the name falls in, and which installed
provider holds that zone. It does this in order:

1. **`--dns-provider`** names a provider directly. The CLI uses it and does not
   ask anything.
2. Otherwise, the CLI asks every installed provider which zones its credential
   can see, and picks the one whose longest matching zone the name falls
   inside.
3. If no installed provider claims the zone — or none is installed, or the
   machine cannot be reached — the answer is **`manual`**: the CLI cannot write
   anywhere, so it prints the record to create by hand instead of failing.
4. If more than one installed provider claims the same zone, the CLI stops and
   asks: say which one with `--dns-provider`.

For example, with `hostinger` and `cloudflare` both installed, and `hostinger`
holding `example.com` while `cloudflare` holds `example.net`:

```
devmachine dns add www.example.com A 198.51.100.10
```

resolves to `hostinger`, because `www.example.com` falls inside the zone
`example.com` and no other installed provider claims it. Every command prints
which provider it picked and why, on stderr, so a surprising answer is never
silent.

The command asks immediately before it writes. For non-interactive use,
`--publish` records that public DNS is intended; `--yes` alone never creates a
record.

## `dns status` is not `dns list`

`dns list` and `dns check` ask the **registrar**: what is actually configured,
whether or not it has taken effect anywhere else yet. That is a provider
question, and it needs a provider installed.

`dns status` asks **the public internet**: resolve the name, fetch its
certificate, make one request — from this computer, the way anybody else would
see it. It needs no provider and no machine at all, because it is not asking
either of them anything; it is asking the internet what they produced.

A record can be correct in `dns list` and still fail `dns status`, because it
has not propagated yet. A record can be correct in `dns status` for a name no
installed provider holds, because somebody set it by hand. They answer
different questions on purpose.
