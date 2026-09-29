# Sharing a login

One GitHub account usually serves every workspace on a machine. Logging
into it six times is six chances to end up on six different accounts, so
the login happens once and the result is copied.

**The CLI generates the copying** — a package never writes it. The CLI is
the only thing that knows both halves: a package says where its tool
keeps a session (`stored_at`) and whether a copy works elsewhere
(`shareable`); the configuration says which workspaces want the shared
one. Put the copying in a package, and every shareable tool grows its own
copy task, written again and slightly differently each time.

## What runs

For each login that resolves to `machine` scope, `sync` generates three
tasks:

1. Look for the master copy under `/etc/devmachine/<name>/`.
2. Create every directory on the way to `stored_at`, owned by the
   account.
3. Copy the master there, owned by the account, mode `0600`.

All three skip when the master is not there — running `sync` before
anybody has run `devmachine login` is normal, and the next run picks the
session up.

Every directory is named, not just the last one: Ansible's own `mkdir -p`
style would make `~/.config` root-owned, breaking the workspace's own
tools for no obvious reason.

## The one that matters: opting out

A workspace that says `gh: own` is **not in the loop at all**.

```yaml
credentials:
  gh: machine

workspaces:
  - name: acme
  - name: bob
    credentials:
      gh: own
```

The copy overwrites `stored_at`. A shared login copied into bob's home
would replace the account he logged in with, with nothing saying why —
so the generated loop carries acme and nobody else, and bob's name
never appears near it.

## What beats what

| Order | Where |
| --- | --- |
| 1 | the workspace's `credentials:` |
| 2 | the configuration's `credentials:` |
| 3 | the package's own `scope:` |

A package that says `shareable: false` beats all three: asking for
`machine` on one is refused by name, because the copy would land, the
tool would reject it, and nothing would say why. A package that has not
said `shareable: true` is treated as one that does not.

Where each credential lives, and how a non-login value is delivered, is
in [Configuration](../concepts/configuration.md#credentials).
