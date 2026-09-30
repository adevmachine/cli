# Set up devmachine with a coding agent

This page is for a coding agent — Claude Code, Codex, Cursor or any other —
that a person asked to set devmachine up for them. If you are that agent,
follow it in order. Talk to the person in their language, ask one question at
a time, and never guess an answer only they know.

devmachine sets up a VPS for the person to code on. Each project gets its own
account on the server, called a workspace, with its own tools, logins and
coding agent. devmachine runs on the person's own computer and reaches the
server over SSH.

## Rules

- **Only the person confirms the server's identity.** `devmachine setup` shows
  a code called the fingerprint and asks if it is trusted. The person checks
  it against their provider's dashboard. Never answer that prompt yourself,
  never pipe an answer into `setup`, never pass a flag that skips it.
- **Only the person types a password or signs in to a service.** When a step
  needs one, hand them the command.
- **Show before you change a server.** Run `devmachine sync --check`, show
  the person what it will do, and run `devmachine sync --yes` only after they
  agree.
- **Use commands, not files.** Change the configuration with `devmachine`
  commands. Never edit `config.yml` by hand.
- When something fails, read the error, then check
  [troubleshooting](troubleshooting.md), before trying anything else.

## 1. Install the CLI

Check with `devmachine version`. If it is missing, run:

```
curl -fsSL https://mydevmachine.sh/install.sh | sh
```

On macOS with Homebrew, that installs through the tap
(`brew install mydevmachine/tap/devmachine`); otherwise it downloads and
verifies the release binary for you.

Then run `devmachine skills add`. It teaches you, and any other agent the
person uses, the whole CLI. This only takes effect in a new session, so keep
following this page either way.

## 2. Ask what you need

Ask one question at a time:

1. The server's address — an IP or a hostname. It must run Debian or Ubuntu.
2. Can they reach it as root over SSH — with a key already on the server, or
   with the root password their provider gave them?
3. A name for the first workspace, for example the project they will work on.
4. Do they want a coding agent inside that workspace? Claude Code is a
   package called `claude-code`.
5. Only if they want an app visible at a URL: a domain, and whether it is at
   Hostinger or Cloudflare. Otherwise skip this.

## 3. Hand over `setup`

Tell the person to run `devmachine setup` themselves — in Claude Code they can
type `! devmachine setup` in this session. Tell them what it will ask: a name
for the server, the address, the login (`root`) and port (`22`), and a domain
(empty is fine). It also asks how the CLI should log in — the first option, a
key of its own, is right when unsure — and shows the fingerprint to check
against the provider's dashboard.

Once the machine answers, it asks two more questions: whether to write SSH
host entries so `ssh <workspace>-devmachine` and `mosh` work from any
terminal — **say yes**, that is what makes the workspace reachable outside
this CLI too, including from an editor like VS Code Remote-SSH — and whether
to reach the machine over Tailscale as well, which is the person's call.

Once they answer, `setup` sets up a key, checks it works, turns off password
logins, and locks in the latest set of packages.

When they say it finished, run `devmachine doctor`. Every line should pass or
warn; fix a warning about SSH aliases with `devmachine aliases --write`, and a
missing credential with the `devmachine login` command it names.

## 4. Create the workspace

```
devmachine workspaces new <name>
devmachine packages add claude-code --workspace <name>
devmachine sync --check
```

The machine already has the `essentials` package from `setup` (base tools,
git, a firewall and Caddy). If they asked for a bare server, tell them to run
`devmachine setup --no-essentials` instead. Skip the `claude-code` line if
they wanted no coding agent. Show them the plan, and
once they agree run `devmachine sync --yes`. The first sync takes a few
minutes.

## 5. Sign in to GitHub

If they use GitHub, hand them `devmachine login gh`. It opens a terminal on
the server with GitHub's own sign-in page, which only they can finish. Then
run `devmachine sync --yes`, which copies that login into every workspace
that uses GitHub.

## 6. Hand it back

Tell them to enter the workspace with `devmachine ssh <name>` — or, if they
said yes to SSH aliases, `ssh <name>-devmachine` works the same way from any
terminal or editor. If they asked for a coding agent, they run `claude`
there once to sign in.

Offer what they may want next, each in one short line:
[guides](guides/index.md) — a site with its own domain and HTTPS, a
Docker app, Claude Code opened from the phone, an agent in its own workspace.
