# Getting started

devmachine sets up a VPS for you to code on. Each project or client gets its
own account on the server, called a workspace, with its own tools and logins.

You need a Debian or Ubuntu VPS you can reach as root over SSH, and a Mac or
Linux computer.

Three commands:

```
curl -fsSL https://mydevmachine.sh/install.sh | sh
devmachine setup
devmachine workspaces new acme && devmachine sync
```

On macOS, that line installs through Homebrew if you have it. Prefer
Homebrew directly? `brew install mydevmachine/tap/devmachine` works the same
way.

`devmachine skills add` teaches your coding agent how to use devmachine. You
can run it before `setup`, too.

Then work in your new workspace:

```
devmachine ssh acme
```

`setup` also asks whether to write SSH host entries for you. Say yes, and
`ssh acme-devmachine` works from any terminal, editor or app that dials `ssh`
directly — VS Code Remote-SSH, Zed, the macOS app. For mosh, use
`devmachine mosh acme`. See [reaching your server](concepts/reaching-your-server.md).

## What each command does

`setup` asks for your server's address. It shows a code called the
fingerprint: check it matches the one in your provider's dashboard, so you
know you are talking to your own server. Then it sets up a key and turns off
password logins, so only you can get in.

`setup` also gives the server the `essentials` package: base tools, git, a
firewall, SSH with passwords kept off, and Caddy to publish sites with
HTTPS. Docker is not in it; add it with `devmachine packages add docker` when
you need it. Want a bare server instead? Run `devmachine setup
--no-essentials`.

`workspaces new acme` creates a workspace called acme. `sync` builds it
all on the server.

**Set up before the essentials existed?** Add them to the server you already
have, nothing to rebuild. What is already installed stays as it is:

```
devmachine packages pin
devmachine packages add essentials
devmachine sync
```

`packages pin` moves you to the latest set of packages, the first one with
`essentials` in it.

A new workspace comes with git, the GitHub CLI, Node LTS (through mise), bun,
and zsh with Oh My Zsh. Every SSH login lands in a tmux session, so a dropped
connection loses nothing: connect again and you are back where you were. Turn
that off with the [`zsh.tmux_auto_attach`](reference/settings.md) setting. The server itself gets
only the packages you add to it.

Something failed? [Troubleshooting](troubleshooting.md) says what each error
really means.

## Set it up to develop

- **A coding agent:** `devmachine packages add claude-code --workspace acme`,
  then `devmachine sync`.
- **GitHub, signed in once for every workspace:** `devmachine login gh`, then
  `devmachine sync`. See [credentials](concepts/credentials.md).
- **Docker:** `devmachine packages add docker`, then let the workspace use it
  with `devmachine workspaces edit acme --set workspace.groups=[docker]`,
  then `devmachine sync`.
- **Anything else:** `devmachine packages list` shows what is available.
  [Packages](concepts/packages.md) says how to add one or write your own.
- **Teach your coding agent this CLI:** `devmachine skills add`. See
  [agent skills](concepts/agent-skills.md).

## Extras

- **Show an app at a URL** — anything listening on a port, a dev server or a
  Docker container. Add a reverse proxy once with
  `devmachine packages add caddy` and `devmachine sync`, then
  `devmachine expose add acme 3000 --host app.example.com` puts it on Caddy
  in seconds. To reach it only from your own computer, with no public
  URL, use `devmachine tunnel acme 3000` instead. See
  [publishing](concepts/publishing.md).
- **Keep your configuration in git:** `devmachine setup git`. See
  [versioning your configuration](how-it-works/versioning-your-configuration.md).

Real setups, step by step: [guides](guides/index.md).
