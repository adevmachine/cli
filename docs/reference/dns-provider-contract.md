# The DNS provider contract

What a DNS provider's entrypoint is called with, and what it must answer
back. Follow this page and the CLI works with your provider on the first
try; guess, and it does not.

`devmachine packages new <name> --scope machine --kind dns` writes an
entrypoint that already follows this contract — `list` returns an empty
list, and `upsert` and `delete` return a clear "not implemented yet"
error. Start from that and replace the placeholders, rather than writing
an entrypoint from nothing.

## How it is called

```
<entrypoint> list|upsert|delete <zone>
```

`<entrypoint>` is the package's `entrypoint`, for example `bin/provider`.
The command is `argv[1]`, the zone is `argv[2]`.

`zones` and `help` take no zone:

```
<entrypoint> zones
<entrypoint> help
```

`zones` answers which zones the credential can see. The CLI uses this to
work out which registrar holds a name, so it has to ask this before it
can even name a zone — and a provider that cannot answer it can never be
chosen.

```json
{"zones": ["example.com", "example.net"]}
```

`help` answers what the entrypoint accepts, and is what `devmachine
packages help <name>` prints. Leave `args` out for a command that takes
none.

```json
{"commands": [
  {"name": "zones",  "summary": "The zones this token can see."},
  {"name": "list",   "summary": "Every record in a zone.", "args": "<zone>"},
  {"name": "upsert", "summary": "Make a name hold exactly one value.", "args": "<zone>"},
  {"name": "delete", "summary": "Remove one value from a name.", "args": "<zone>"},
  {"name": "help",   "summary": "This list."}
]}
```

Both `zones` and `help` are required. A manifest that lists a command its
entrypoint actually refuses is a lie no validator can catch, because only
the entrypoint knows.

## What it receives

`list` takes nothing beyond the zone.

`upsert` and `delete` take one record as JSON **on stdin**:

```json
{"name": "www", "type": "A", "value": "198.51.100.10", "ttl": 300}
```

- `name` is a **label**, never a full name: `www`, or `@` for the apex
  (the root domain of your server). The provider adds the zone itself.
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

`kind` must be one of exactly these:

| `kind` | When |
| --- | --- |
| `zone_not_found` | The zone does not exist, or the token cannot see it. |
| `unauthenticated` | The token was rejected. |
| `forbidden` | The token can see the zone but cannot change it. |
| `invalid_record` | The record was rejected — a bad type, a bad value, a name the registrar refuses. |
| `rate_limited` | The registrar's API is throttling this token. |
| `ambiguous` | The name holds several values and the request does not say which one. |

A `kind` outside this list is treated as a bug in the provider, not a new
kind the CLI learns.

## What the environment holds

Whatever `/etc/devmachine/<credential>/env` sets for this provider's
credential, exported, **and nothing else the CLI adds**. A provider that
reads a variable its own credential does not declare will work on the one
machine that happens to have it set, and break on everybody else's.

## Where and as whom it runs

On the machine, as root, from `/opt/devmachine/roles/<package>/`. Its
working directory is not guaranteed, so use absolute paths — never a path
relative to the entrypoint's own location.

## The rules that matter most

- `upsert` makes the name hold **exactly** that one value. Adding to
  whatever is already there is wrong, even when the registrar's API makes
  that the easy path.
- `delete` removes exactly that one value and leaves every other value at
  that name untouched.
- An empty `value` on either call means the whole set at that name and
  type, not a value that happens to be an empty string.

## Python 3, standard library only

Ansible already needs Python on any machine this CLI sets up, so Python 3
is always there, and an entrypoint with no extra dependencies always
works. No `requests`, no `pip install`, no shelling out to `jq` — a
provider that needs a package the machine does not have will fail during
`sync`, far from wherever that dependency was declared.

## `format` in `package.yml`

`format: 1` lets an older CLI refuse a package it cannot read cleanly,
instead of misreading a field that changed shape. As a provider author,
you never have to think about it beyond leaving the number the skeleton
already put there.

## Never retry a rate limit

A provider that hits a rate limit reports `rate_limited` and stops.
Retrying inside the entrypoint hides how throttled the registrar really
is from whatever called it, and turns one slow request into a hang with
no visible cause.
