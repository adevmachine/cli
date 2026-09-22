# The DNS provider contract

What a DNS provider's entrypoint is called with, and what it must answer. A
provider that follows this page works with the CLI on the first try; a
provider that guesses does not.

`devmachine packages new <name> --scope machine --kind dns` writes an
entrypoint that already answers this contract — an empty list from `list`, and
a clear "not implemented yet" error from `upsert` and `delete`. Start there and
replace the placeholders, rather than writing an entrypoint from nothing.

## How it is called

```
<entrypoint> list|upsert|delete <zone>
```

`<entrypoint>` is the package's `entrypoint`, for example `bin/provider`. The
command is `argv[1]`, the zone is `argv[2]`.

`zones` and `help` take no zone:

```
<entrypoint> zones
<entrypoint> help
```

`zones` answers which zones the credential can see. It is how the CLI works
out which registrar holds a name, so asking it for a zone first would be
circular, and a provider that cannot answer it can never be chosen.

```json
{"zones": ["example.com", "example.net"]}
```

`help` answers what the entrypoint accepts, and is what `devmachine packages
help <name>` prints. `args` is left out for a command that takes none.

```json
{"commands": [
  {"name": "zones",  "summary": "The zones this token can see."},
  {"name": "list",   "summary": "Every record in a zone.", "args": "<zone>"},
  {"name": "upsert", "summary": "Make a name hold exactly one value.", "args": "<zone>"},
  {"name": "delete", "summary": "Remove one value from a name.", "args": "<zone>"},
  {"name": "help",   "summary": "This list."}
]}
```

Both are mandatory. A manifest that lists a command its entrypoint refuses is
a lie no validator can catch, because only the entrypoint knows.

## What it receives

`list` takes nothing beyond the zone.

`upsert` and `delete` take one record as JSON **on stdin**:

```json
{"name": "www", "type": "A", "value": "198.51.100.10", "ttl": 300}
```

- `name` is a **label**, never a full name: `www`, or `@` for the apex. The
  provider adds the zone itself.
- `ttl` of `0` means "choose": use whatever the registrar defaults to.

## What it must print

On success, one JSON document on stdout:

- `list` → `{"records": [{"name": "...", "type": "...", "value": "...", "ttl": 300}, ...]}`
- `upsert` and `delete` → `{}`

## What a failure looks like

Exit non-zero, and print one JSON document on stdout:

```json
{"error": {"kind": "zone_not_found", "message": "no zone answers for example.com"}}
```

`kind` is one of exactly these, in full:

| `kind` | When |
| --- | --- |
| `zone_not_found` | The zone does not exist, or the token cannot see it. |
| `unauthenticated` | The token was rejected. |
| `forbidden` | The token can see the zone but cannot change it. |
| `invalid_record` | The record was rejected — a bad type, a bad value, a name the registrar refuses. |
| `rate_limited` | The registrar's API is throttling this token. |
| `ambiguous` | The name holds several values and the request does not say which one. |

A `kind` outside this list is treated as a bug in the provider, not a new kind
the CLI learns.

## What the environment holds

Whatever `/etc/devmachine/<credential>/env` sets for this provider's
credential, exported, **and nothing else the CLI adds**. A provider that reads
a variable its own credential does not declare works on the machine that
happens to have it set and breaks on everybody else's.

## Where and as whom it runs

On the machine, as root, from `/opt/devmachine/roles/<package>/`. Its working
directory is unspecified — use absolute paths, never a path relative to the
entrypoint's own location.

## The semantics, copied from the interface

- `upsert` makes the name hold **exactly** that one value. Appending to
  whatever is already there is the wrong implementation even when the
  registrar's API makes appending the easy path.
- `delete` removes exactly that one value and leaves the rest of the name's
  values untouched.
- An empty `value` on either call means the whole set at that name and type,
  not a value that happens to be the empty string.

## Python 3, standard library only

Ansible already requires Python on any machine this CLI provisions, so Python
3 is always there and an entrypoint with no dependencies cannot break on
install. No `requests`, no `pip install`, no `jq` shelled out to — a provider
that needs a package the machine does not have is a provider that fails on
`sync`, far from wherever the dependency was declared.

## `format` in `package.yml`

`format: 1` is what lets an older CLI refuse a package it cannot read, cleanly,
rather than misreading a field that changed shape. A provider never has to
think about it beyond writing the number the skeleton already put there.

## Never retry a rate limit

A provider that hits a rate limit reports `rate_limited` and stops. Retrying
inside the entrypoint hides how slow the registrar really is from whatever
called it, and turns one throttled request into a hang nobody can see the
cause of.
