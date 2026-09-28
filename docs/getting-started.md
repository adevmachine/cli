# Getting started

This guide takes you from nothing to a workspace you can `ssh` into, with a
coding agent ready inside it, and a site published on your own domain. No
prior devmachine knowledge needed.

## 1. What you need

- A VPS with root SSH access, running Debian or Ubuntu. Any provider.
- This computer, running macOS or Linux.
- Optional: a domain name, and an account with a DNS provider devmachine has
  a package for — Hostinger or Cloudflare both work (see
  [DNS](concepts/dns.md)). Without a domain you can still do everything
  except publish a site under your own hostname.

## 2. Install

```
brew install adevmachine/tap/devmachine
```

On Linux without Homebrew, download the tarball for your platform from the
[releases page](https://github.com/adevmachine/cli/releases) and put the
`devmachine` binary on your `PATH`.

Check it worked:

```
devmachine version
```

## 3. Take over the machine

Your VPS arrives with root reachable by SSH, a password, and no key. One
command changes that:

```
devmachine setup
```

It asks four things: where the machine is, who the administrative account is
(usually `root`), which port, and a domain (optional, used later for
publishing).

Then it shows the server's SSH host-key fingerprint and asks you to confirm
it. **Check this against your provider's console before you accept.** A
fingerprint you did not compare is trust on first use, not proof the machine
is the one you think it is.

Next it asks how to log in: a new key it generates for you (the safe default
if you have no opinion), a key file already on this computer, or a key your
SSH agent holds.

From there `setup` works on its own:

- it tries the key first, and only asks for the root password if the key
  does not already work;
- once the key is installed, it opens a **new** connection with the key
  alone to prove it works, before changing anything else;
- only after that proof does it turn password login off;
- it installs Ansible, which is the last thing it does by hand.

If the proof fails, password login stays on and you can still get in — see
[the trust bootstrap](how-it-works/trust-bootstrap.md) for why the order
matters. Pass `--no-harden` to install and prove the key but leave password
login on.

You know it worked when `setup` finishes with no error and prints the
machine it added.

## 4. Check it

```
devmachine doctor
```

```
pass  configuration       machine "main", 1 address(es), admin root, port 22, 0 workspace(s)
pass  connection          connected through 203.0.113.10
pass  operating system    ubuntu
pass  ansible             ansible-playbook 2.16.3
```

Every row must say `pass`. A `fail` names what is wrong and the command that
fixes it.

## 5. Your first workspace

A **workspace** is one Linux account on one machine — where a person works.
A **package** is a recipe for one thing a machine or a workspace needs, from
a shell to a coding agent. See
[Machines and workspaces](concepts/machines-and-workspaces.md) and
[Packages](concepts/packages.md) for the full picture.

Make one, then build it:

```
devmachine workspaces new alice
devmachine sync
devmachine ssh alice
```

`workspaces new` only edits your local configuration — it creates no
account yet. `sync` reads that configuration and makes the machine match
it: this is the step that actually creates the Linux user `alice` and
installs its packages. `ssh alice` lands you in a shell as that user.

You know it worked when the prompt shows you logged in as `alice` on the
machine.

## 6. Sign in once, share everywhere

Some tools need a real login — GitHub's `gh`, for example — and nobody can
automate a browser sign-in. devmachine gets you into the right session and
spreads the result to every workspace that needs it:

```
devmachine login gh
devmachine credentials push
```

`login` opens a real terminal session for that tool's own sign-in flow.
`credentials push` copies the result to every workspace that shares it. Run

```
devmachine credentials list
```

to see what is signed in and what each missing row needs. See
[Credentials](concepts/credentials.md) for the difference between a shared
login and one each workspace keeps for itself.

## 7. Publish something

Say `alice` has a web app listening on port 3000 inside her workspace, and
you own `example.com`. Publish it at `app.example.com`:

```
devmachine expose add alice 3000 --host app.example.com
```

Before it records anything, `expose add` asks you to confirm: anybody who
learns `app.example.com` will be able to reach whatever is on that port.
Answer only after you are sure that is fine — a review app or a personal
tool is a fine answer, a database admin panel is not. See
[Publishing](concepts/publishing.md) for what should never go through
`expose`.

It also points the hostname at your machine — writing the DNS record itself
if you installed a provider package, or printing the record to create by
hand if you did not.

Nothing reaches the internet yet. Send it to the machine:

```
devmachine sync
```

Check the result:

```
devmachine expose list
```

`published` means the site is live. `pending` means `sync` has not run yet.
`differs` means the machine does not match the configuration — run `sync`
again. See [how a published site is kept](how-it-works/published-sites.md)
and [DNS](concepts/dns.md) for what is happening behind each state.

## 8. Keep it in git

Your configuration — machines, workspaces, packages — is worth keeping in
version control, so it can be reviewed and restored. Never your keys: those
stay out on purpose.

```
devmachine setup git
```

This makes your configuration directory a git repository and, with `gh`
installed and signed in, offers to create a **private** remote and push to
it. See
[versioning your configuration](how-it-works/versioning-your-configuration.md)
for exactly what gets committed and why the remote must stay private.

## 9. Teach your coding agent

devmachine ships Agent Skills: operational knowledge for Claude Code, Codex,
Pi and OpenCode, so your agent knows how this machine works instead of
guessing.

Install them on this computer:

```
devmachine skills add
```

Give a workspace the same knowledge:

```
devmachine packages add devmachine-skills --workspace alice
devmachine sync --tags devmachine-skills
```

`skills list` shows what is installed and where. See
[Agent Skills](concepts/agent-skills.md) for how the canonical copy and the
per-harness links work.

## 10. Where to go next

- [Commands](reference/commands.md) — every command and flag, in full.
- [Machines and workspaces](concepts/machines-and-workspaces.md),
  [Packages](concepts/packages.md), [Credentials](concepts/credentials.md),
  [DNS](concepts/dns.md), [Publishing](concepts/publishing.md) — the ideas
  behind what you just did.
- [Troubleshooting](troubleshooting.md) — when a command fails, look here
  first. It lists the exact errors you are likely to meet and what each one
  really means.
