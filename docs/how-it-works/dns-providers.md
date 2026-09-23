# DNS providers

Every fact here exists so somebody learns it from this page, not by damaging a
zone. Read it before you touch a real one.

## A provider is a package, not a compiled-in vendor

Talking to Hostinger or Cloudflare is an ordinary package: `devmachine packages
add hostinger`, run on the machine, never on your computer. There is no vendor
list inside the CLI binary and no version of it that "supports" a registrar the
package repository does not.

The safety property that falls out: **a provider nobody installed cannot be
written to.** There is no code path that reaches a registrar's write API unless
its package is on the lock file for that machine. Adding a provider is a
decision with a diff; the absence of one is not a missing feature, it is a
missing credential and a missing recipe, both visible.

## What the provider gets from the shell

`PATH`, `HOME`, and the one token its own credential declares — nothing else.
The CLI sources `/etc/devmachine/<credential>/env` with `set -a; ... ; set +a`
immediately before running the entrypoint, so nothing else from the operator's
shell or the machine's environment leaks in. A provider that reads a variable
its own credential does not declare works by accident, on the one machine that
happens to have it set, and breaks on everybody else's.

The token itself never appears in a command argument, where `ps` would show it
to every account on the machine. Only the credential's name does, and that name
is not a secret.

## Hostinger

### RRsets, not records

Hostinger's API has no idea of a single record. It has **RRsets**: one entry
per `(name, type)`, holding a list of values. `www A` is one RRset that can
carry several addresses at once. The provider hides this — `list` flattens
each RRset into one entry per value — but every write has to work in RRset
terms, and that shapes everything below.

### `overwrite: false` appends

Hostinger's `PUT` takes a whole RRset and an `overwrite` flag. With `overwrite:
false`, the values in the request are **added** to whatever is already there.
Pointing an existing name at a new address with `overwrite: false` alone does
not replace the old one — it leaves both, a round-robin nobody asked for and a
name that resolves correctly only half the time.

### `overwrite` defaults to `true`, so it is always sent explicitly

Leave the field out and Hostinger's own default is `overwrite: true`, the
destructive mode. The provider never relies on that default: every `PUT` it
sends carries `overwrite` explicitly, `true` or `false`, decided by the code,
not by whatever Hostinger happens to default to today. A test asserts this on
**every** `PUT` body the fake server saw in a run, not only the last one — a
provider that gets the first call right and the second wrong would still pass a
test that only checked the end state.

### Replacing a value: delete, then append — not one overwriting call

Changing what a name already points at needs `upsert` to hold **exactly** the
new value, not the new value plus the old one. Hostinger's API gives two ways
to reach that:

- One `PUT` with the whole new value and `overwrite: true`.
- A `DELETE` of the RRset, then a `PUT` of the new value with `overwrite:
  false`.

The provider uses the second, and it costs two calls where the first would cost
one. Hostinger's current OpenAPI description says `overwrite: true` replaces
the existing records with the records in the payload. A one-RRset payload would
therefore empty the rest of the zone — every other RRset, gone. The single-call
path is not an optimization waiting for a test; it has the wrong blast radius.

The two-call path is verified: the `DELETE` removes only the named RRset by an
explicit filter, and the `PUT` afterwards only ever writes what the provider
already means to write. It has a real cost — **a name holds nothing for the
gap between the two calls** — a resolver asking in that window gets NXDOMAIN
for that name and type. That window was still the better trade than a write
whose documented blast radius is the whole zone.

### A DNS-only token needs its zones configured

Hostinger's DNS API answers about one zone at a time and has no list-zones
endpoint. Automatic provider selection normally gets the account's domains
from the Domains portfolio endpoint, but a least-privilege DNS token receives
403 there even though it can read its DNS zone. Set `hostinger.zones` on the
machine for that case; the provider answers from the configured list and never
asks the portfolio. An empty list preserves portfolio discovery for broader
tokens.

### A delete filter takes the whole RRset

Hostinger's `DELETE` removes an entire `(name, type)`, not one value inside it.
Deleting one value out of several therefore means: read the RRset, delete it
whole, then `PUT` the values you meant to keep back with `overwrite: false`.
That is not atomic — the same gap as above applies — but it is the only way to
remove one value without the API doing it for you.

### Writes are asynchronous

A successful `PUT` or `DELETE` answers "request accepted," not "done." A `list`
called immediately after can still show the state from before the write. This
is why the provider does not verify its own writes by reading them back
straight away — a `list` racing the write would report a real change as a
failure. Give it a moment before checking with `dns status` or `dns list`.

## Cloudflare

### `PUT` clears every field it was not given

Cloudflare's DNS record `PUT` replaces the whole record. Send only `content`
and `ttl`, and `proxied`, `comment` and `tags` reset to their defaults — a
proxied record silently stops being proxied, a comment somebody wrote by hand
disappears. The provider never uses `PUT` for a change. It uses `PATCH`, which
touches only the fields in the request and leaves the rest of the record alone.

### No upsert: every write past a create needs an id

Cloudflare has no "make this name hold this value" call. Creating a record
returns an id; changing or deleting one needs that id, and the only way to get
it is to list first. `upsert` therefore always lists before it writes: create
on no match, `PATCH` by id on exactly one match, and refuse to guess when more
than one record matches the name and type (`ambiguous`).

### An invisible zone is a 200, not a 404

Ask Cloudflare for a zone the token cannot see, or one that does not exist, and
the answer is `200` with an empty `result` array — not an error status. The
provider treats an empty result as `zone_not_found` itself; trusting the HTTP
status alone here would treat a zone that plainly did not resolve as success.

### `proxied` is always `false`

The provider never sets a record to be proxied through Cloudflare's network.
Certificates on this machine renew through the HTTP-01 challenge, which needs
the challenge request to reach the machine directly. A proxied record answers
from Cloudflare's edge instead, and the challenge fails.

## Neither provider retries a rate limit

A `429` is reported as `rate_limited` and the provider stops — it never sleeps
and tries again on its own. Retrying inside the entrypoint would hide from
whatever called it how throttled the registrar really is, and could turn one
throttled request into a hang with no visible cause. Cloudflare in particular
blocks every call from a token for five minutes after a single `429`, so a
retry loop there would not even help.
