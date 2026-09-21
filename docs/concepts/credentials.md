# Credentials

Every tool a workspace uses needs to be signed in: `gh`, `claude`, a DNS
provider's token, an AWS key. That is the part of setting up a machine which
does not survive a rebuild, because it is the part nobody wrote down.

A workspace with its tools installed and none of them signed in is only half
created. Credentials are how the CLI knows that, and how it fixes what it can.

## Three kinds, and only one of them is hard

| Kind | Examples | Can it be automated? |
| --- | --- | --- |
| **manual** | `gh`, `claude`, `tailscale up` | **no** — a browser or a device code, and a person |
| **secret** | an API token, an SMTP password | yes |
| **file** | a VPN profile, a kubeconfig | yes |

**Nobody automates a browser login, and this CLI does not pretend to.** Its job
is to make the login happen in the right place, and to spread the result where
it belongs. See [why a login cannot be
automated](../how-it-works/why-a-login-cannot-be-automated.md).

## The package says how; you supply only a value

A recipe declares what its tool cannot work without, **and how each one is
obtained**. Nobody else knows that `claude` signs in with `claude /login` and
leaves its session in `~/.claude/.credentials.json` — the package that installs
Claude does.

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
`devmachine secrets`, not in a file you might commit. What the configuration may
hold is one preference per credential: whether to share it.

## Scope is per credential, because reality is per credential

One GitHub account usually serves every workspace, so signing in once and
copying the result is right. Claude accounts usually differ, so there is no
master session to copy and each workspace signs in for itself.

That is a property of the tool and of how you use it, not a setting somebody
picked. So the package recommends a `scope:`, and **you decide** — see
[sharing a login](../how-it-works/sharing-a-login.md).

**`shareable: true` is a claim about one tool that has been tested, never a
default.** A token bound to a device, or to a browser session, does not survive
being copied: the file lands where the tool looks, the tool rejects it, and
nothing on the machine says why. Which tools tolerate a copy is found by trying,
not by reasoning, so a package that has not been tried says `false` and the CLI
refuses to share it.

## The commands

```
devmachine credentials list                    what is declared, what is there, and the command that fixes each
devmachine login <name> [--workspace w]        sit through the tool's own login, in the right place
devmachine secrets set <name>                  hand over a value
devmachine credentials push                    deliver the values and files
```

`credentials list` is the one that earns its place: every row carries the
command that resolves it, so nobody has to work out what to do next.

## Where a credential lands

- A **machine** credential lives in `/etc/devmachine/<name>/`, root-owned, mode
  0600. `sync` copies it into each workspace that wants it.
- A **workspace** credential lives in that workspace's home, owned by that
  account. The home is 0750, so no other workspace can read it.

A secret delivered to a machine is on that machine's disk. That is what the
tools expect to find, and it is what works for a process, a container and a
systemd unit alike. The alternatives either vanish when a shell closes or need
an agent running on the machine, which this project decided not to have.

## What `secrets` is, and is not

**A staging area, not a vault.** You type a value once, `credentials push`
delivers it to the machine, and the local copy is a convenience for adding a
second machine without retyping. `devmachine secrets rm` removes it.

Nothing needs a secret to live on your own computer: a DNS provider runs on the
machine and reads the credential delivered there.

## A `.env` with several keys

One credential is one value, so three keys are three credentials, each in its
own `/etc/devmachine/<name>/env`, and the recipe composes the file in its own
tasks.

The alternative — giving a credential a key name — puts file formats in the CLI,
which is the beginning of it knowing what `.env`, `.ini` and `.toml` are.
