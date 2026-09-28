# Set up devmachine with a coding agent

This page is written for a coding agent — Claude Code, Codex, Cursor or any
other — that a person asked to set devmachine up for them. If you are that
agent, follow it in order. Talk to the person in their language, ask one
question at a time, and never guess an answer that only they know.

devmachine turns a VPS into workspaces the person develops in: one Linux
account per project, each with its own tools, logins and coding agent. It runs
on the person's own computer and reaches the VPS over SSH.

## Rules

- **Only the person confirms the server's host key.** `devmachine setup` shows
  a fingerprint and asks whether to trust it. The person compares it with their
  provider's console. Never answer that prompt, never pipe answers into
  `setup`, never pass flags that skip it.
- **Only the person types a password or signs in to a service.** When a step
  needs one, hand the command to them.
- **Show before changing a machine.** Run `devmachine sync --check`, show the
  person what it will do, and run `devmachine sync --yes` only after they agree.
- **Use commands, not files.** Change the configuration with `devmachine`
  commands, never by editing `config.yml`.
- When something fails, read the error, then
  [troubleshooting](troubleshooting.md), before trying anything else.

## 1. Install the CLI

Check with `devmachine version`. If it is missing, on macOS run
`brew install adevmachine/tap/devmachine`; on Linux, take the binary for the
platform from https://github.com/adevmachine/cli/releases and put it on the
`PATH`.

Then run `devmachine skills add`. It teaches you, and any other agent the
person uses, the whole CLI. A skill may only load in a new session; keep
following this page either way.

## 2. Ask what you need

Ask, one at a time:

1. The VPS address — an IP or a hostname. It must run Debian or Ubuntu.
2. Whether they can reach it as root over SSH, with a key already on the
   server or with the root password their provider gave them.
3. A name for the first workspace, for example the project they will work on.
4. Whether they want a coding agent inside that workspace. Claude Code is a
   package: `claude-code`.
5. Only if they want to show an app at a URL: a domain, and whether it is at
   Hostinger or Cloudflare. Otherwise skip this.

## 3. Hand over `setup`

Tell the person to run `devmachine setup` themselves — in Claude Code they can
type `! devmachine setup` in this session. Tell them what it will ask: a name
for the machine, the address, the login (`root`) and port (`22`), and a domain
(empty is fine). It also asks how the CLI should log in — the first option, a
key of its own, is right when unsure — and shows a host key fingerprint to
compare with the provider's console.

Once they answer, `setup` installs the key, checks it works, turns password
login off, and pins the latest packages release.

When they say it finished, run `devmachine doctor`. Every line should pass.

## 4. Create the workspace

```
devmachine workspaces new <name>
devmachine packages add claude-code --workspace <name>
devmachine sync --check
```

Skip the second line if they wanted no coding agent. Show the plan, and after
they agree run `devmachine sync --yes`. The first sync takes a few minutes.

## 5. Sign in to GitHub

If they use GitHub, hand them `devmachine login gh` — it opens a terminal on
the machine with GitHub's own sign-in, a device code only they can finish.
Then run `devmachine sync --yes`, which copies that login into every workspace
that uses GitHub.

## 6. Hand it back

Tell them to enter the workspace with `devmachine ssh <name>`. If they asked
for a coding agent, they run `claude` there once to sign in.

Offer what they may want next, each one short:
[examples](examples/index.md) — a site with its own domain and TLS, a Docker
app, Claude Code opened from the phone, an agent in its own workspace.
