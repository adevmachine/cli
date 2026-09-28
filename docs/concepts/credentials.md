# Credentials

Every tool a workspace uses needs to be signed in: `gh`, `claude`, a DNS
provider's token, an AWS key. Signing in is the part of setting up a
machine that a rebuild wipes out, because it's the part nobody wrote down.
Credentials are how the CLI tracks that, and fixes what it can.

## Three kinds, and only one is hard

| Kind | Examples | Can it be automated? |
| --- | --- | --- |
| **manual** | `gh`, `claude`, `tailscale up` | **no** — a browser or a device code, and a person |
| **secret** | an API token, an SMTP password | yes |
| **file** | a VPN profile, a kubeconfig | yes |

**Nobody automates a browser login, and this CLI doesn't pretend to.** Its
job is to make the login happen in the right place, and spread the result
where it belongs. See
[why a login can't be automated](../how-it-works/why-a-login-cannot-be-automated.md).

## The package says how; you supply only a value

A package declares what its tool needs, and how to get it. Nobody else
knows that `claude` signs in with `claude /login` and leaves its session
in `~/.claude/.credentials.json` — the package that installs Claude does:

```yaml
# packages/claude-code/package.yml
credentials:
  - name: claude
    kind: manual
    scope: workspace
    shareable: true
    command: claude /login
    stored_at: ~/.claude/.credentials.json
```

Your configuration holds **no credential value, ever**. A value goes in
`devmachine secrets`, never in a file you might commit. All it can hold is
one choice: whether to share the login.

## Scope is per credential

One GitHub account usually serves every workspace, so signing in once and
copying it is right. Claude accounts usually differ, so each workspace
signs in for itself. That's a fact about the tool, not a setting somebody
picked — the package suggests a scope, and **you decide**. See
[sharing a login](../how-it-works/sharing-a-login.md).

**`shareable: true` means one tool that's been tested, never a default.**
A login tied to one device or browser doesn't survive being copied: the
file lands where the tool looks, the tool rejects it, and nothing says
why. So an untested package says `false`, and the CLI won't share it.

## The commands

```
devmachine credentials list                    what is declared, what is there, and the command that fixes each
devmachine login <name> [--workspace w]        sit through the tool's own login, in the right place
devmachine secrets set <name>                  hand over a value
devmachine credentials push                    deliver the values and files
```

`credentials list` earns its place: every row carries the command that
fixes it, so you always know what to do next.

## Where a credential lands

- A **machine** credential lives in `/etc/devmachine/<name>/`, readable
  only by root. `sync` copies it into every workspace that wants it.
- A **workspace** credential lives in that workspace's home folder, which
  no other workspace can read.

A secret ends up on the machine's disk, because that's where the tools
expect to find it — for a process, a container, or a background job alike.
Nothing needs a secret on your own computer.

## What `secrets` is, and is not

**A staging area, not a vault.** You type a value once, `credentials push`
delivers it to the machine, and the local copy just saves retyping it for
a second machine. `devmachine secrets rm` removes it.

## A `.env` with several keys

One credential is one value, so three keys are three credentials, each in
its own `/etc/devmachine/<name>/env`. The alternative — giving a
credential a key name — would put file formats like `.env`, `.ini`, and
`.toml` inside the CLI itself.
