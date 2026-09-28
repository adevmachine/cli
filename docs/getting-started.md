# Getting started

devmachine sets up a VPS for you to code on. Each project or client gets its
own account on the server, called a workspace, with its own tools and logins.

You need a Debian or Ubuntu VPS you can reach as root over SSH, and a Mac or
Linux computer.

Three commands:

```
brew install adevmachine/tap/devmachine
devmachine setup
devmachine workspaces new alice && devmachine sync
```

On Linux without Homebrew, get the binary from the
[releases page](https://github.com/adevmachine/cli/releases).

`devmachine skills add` teaches your coding agent how to use devmachine. You
can run it before `setup`, too.

Then work in your new workspace:

```
devmachine ssh alice
```

## What each command does

`setup` asks for your server's address. It shows a code called the
fingerprint: check it matches the one in your provider's dashboard, so you
know you are talking to your own server. Then it sets up a key and turns off
password logins, so only you can get in.

`workspaces new alice` creates a workspace called alice. `sync` builds it on
the server.

A new workspace comes with git, the GitHub CLI, Node LTS (through mise), bun
and zsh. The server itself gets nothing else until you add packages to it.

Something failed? [Troubleshooting](troubleshooting.md) says what each error
really means.

## Set it up to develop

- **A coding agent:** `devmachine packages add claude-code --workspace alice`,
  then `devmachine sync`.
- **GitHub, signed in once for every workspace:** `devmachine login gh`, then
  `devmachine sync`. See [credentials](concepts/credentials.md).
- **Docker:** `devmachine packages add docker`, then let the workspace use it
  with `devmachine workspaces edit alice --set workspace.groups=[docker]`,
  then `devmachine sync`.
- **Anything else:** `devmachine packages list` shows what is available.
  [Packages](concepts/packages.md) says how to add one or write your own.
- **Teach your coding agent this CLI:** `devmachine skills add`. See
  [agent skills](concepts/agent-skills.md).

## Extras

- **Show an app at a URL** — anything listening on a port, a dev server or a
  Docker container. Add a reverse proxy once with
  `devmachine packages add caddy`, then
  `devmachine expose add alice 3000 --host app.example.com` and
  `devmachine sync`. To reach it only from your own computer, with no public
  URL, use `devmachine tunnel alice 3000` instead. See
  [publishing](concepts/publishing.md).
- **Keep your configuration in git:** `devmachine setup git`. See
  [versioning your configuration](how-it-works/versioning-your-configuration.md).

Real setups, step by step: [examples](examples/index.md).
