# Credentials

A credential is a tool being signed in: `gh`, `claude`, a DNS provider's
token. See what's signed in, and what's missing, with:

```
devmachine credentials list
```

## Three kinds

| Kind | Examples | You do it yourself? |
| --- | --- | --- |
| **manual** | `gh`, `claude`, `tailscale up` | **yes** — a browser or a device code |
| **secret** | an API token, an SMTP password | no, the CLI stores it |
| **file** | a VPN profile, a kubeconfig | no, the CLI stores it |

A manual login can't be done for you. See
[why a login can't be automated](../how-it-works/why-a-login-cannot-be-automated.md).

## The commands

```
devmachine credentials list                    what is declared, what is there, and the command that fixes each
devmachine login <name> [--workspace w]        sit through the tool's own login, in the right place
devmachine secrets set <name>                  hand over a value
devmachine credentials push                    deliver the values and files
```

`credentials list` names, for each row, the exact command that fixes it.

## Your configuration holds no values

A value you type goes to `devmachine secrets`, never into a file you'd
commit. `devmachine secrets set cloudflare_token` asks for it without
echoing it back; `devmachine secrets list` shows only names, never
values; `devmachine secrets rm` removes one.

## Sharing a login across workspaces

Some tools, like GitHub, work fine signed in once and shared by every
workspace. Others, like Claude, need each workspace to sign in for
itself. A package suggests which fits its tool, and you can override it
per workspace:

```yaml
credentials:
  gh: machine          # shared, the default here

workspaces:
  - name: bob
    credentials:
      gh: own          # bob signs in for himself
```

See [sharing a login](../how-it-works/sharing-a-login.md) for what
sharing actually copies.

## Where a credential lands

- A **machine** credential is copied into every workspace that shares it.
- A **workspace** credential lives only in that workspace's own home
  folder, readable by nobody else.

## A `.env` with several keys

One credential is one value. Three keys in one `.env` file are three
separate credentials, combined by the package when it writes the file.

## Your app's own secrets

A credential above is declared by a package: something a *tool* needs.
Your own app's secrets — an API key your code reads, a database
password — are not that. Nothing has to declare them:

```
devmachine secrets set STRIPE_KEY --workspace alice
```

This asks for the value the same way `secrets set` always does, stores
it under `alice/STRIPE_KEY` in the keychain, and remembers to deliver
it on the next `devmachine credentials push` — or right away, with
`--push`.

By default it lands in `~/.devmachine/env`, a file the workspace's
shell sources on login (see
[the `~/.devmachine/env` contract](../reference/commands.md#the-devmachineenv-file)
in the command reference). Give `--env-file` a path instead, relative
to the workspace's home, to edit that file directly:

```
devmachine secrets set STRIPE_KEY --workspace alice --env-file app/.env
```

**This edits a real file on the machine.** `credentials push` opens
`app/.env` inside `alice`'s home, replaces the `STRIPE_KEY=` line if
one is there or appends it if not, and leaves every other line —
comments included — exactly as it was. The first time it touches a
file that already existed, it keeps a copy at `app/.env.devmachine.bak`
next to it, so one bad push is not the only copy of what was there
before.

To move a secret to another file, set it again with the new
`--env-file` (or with none, for the default). The next push takes its
line out of the old file before it writes the new one, so the old file
does not keep handing out a value nobody manages any more.

`devmachine secrets list --workspace alice` shows the name and the
target, never the value. `devmachine secrets rm STRIPE_KEY --workspace
alice --from-file` removes the stored value and, on the next push,
the line from the file too.
