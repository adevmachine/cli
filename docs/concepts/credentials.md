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
