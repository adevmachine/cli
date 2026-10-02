---
description: "Install the Pi coding agent in a workspace and use it for everyday work on your server."
category: Agents
level: Beginner
needs:
  - "A workspace"
related:
  - hermes-agent-in-its-own-sandbox.md
  - opencode.md
  - keep-sessions-running.md
  - shared-skills-for-every-workspace.md
---
# Pi as your coding agent

[Pi](https://pi.dev) is an open-source terminal coding agent. It works like
Claude Code or Codex: you point it at a project folder and it reads, edits
and runs code for you, switching between many AI providers as you like.
This guide installs Pi in a workspace so it runs on your server, not your
laptop, and keeps working after you close the terminal.

**You need:** a workspace — see [getting started](../getting-started.md).

## Before you start: machine, skills, workspace

```
curl -fsSL https://mydevmachine.sh/install.sh | sh
devmachine setup
devmachine skills add
devmachine workspaces new acme
devmachine sync
```

Already have a machine? Skip `setup`. Already have the workspace? Skip the
last two. See [getting started](../getting-started.md) for what each command
does.

## By hand

### 1. Add the package

```
devmachine packages add pi --workspace acme
devmachine sync
```

`sync` installs Pi's standalone release, not the npm package: one native
program in `~/.local/share/pi`, linked from `~/.local/bin/pi`. It needs no
Node, and a Node upgrade cannot break it. It also runs as a process called
`pi`, which is how the Devmachine app tells a Pi session from any other.

### 2. Sign in, once

```
devmachine login pi --workspace acme
```

This starts `pi` in a real terminal. Pi has no separate login command: type
`/login` inside it to connect a subscription or an API key. Pi supports
many providers — Anthropic, OpenAI, Google, and others — and you can
switch models later with `/model`.

### 3. Start working

```
devmachine ssh acme
cd ~/dev/my-project
pi
```

Pi reads any `AGENTS.md` in the project for instructions, the same way other
agents do. Close the terminal any time — `devmachine ssh acme` brings you
back to the same tmux session, with Pi still running.

## Teach Pi the devmachine CLI

`devmachine skills add` also teaches Pi how to run devmachine commands, on
your own computer:

```
devmachine skills add --agent pi
```

This helps in a session on your computer where you ask Pi to manage your
machines and workspaces — not inside the workspace where Pi itself runs.

## With your agent

Open a session on your own computer (`devmachine skills add --agent pi`, or
the plain `devmachine skills add`, taught it the CLI) and say:

```text
Install the Pi coding agent in my devmachine workspace acme.
```

The agent adds the `pi` package and runs `sync`. Signing in with `/login`
is yours to do — that is your own account.

**Check it:** `devmachine run --workspace acme -- pi --version` prints a
version. Then start `pi` in a project folder and ask it a question about
the code there.

Source: [pi.dev](https://pi.dev/), [Pi coding agent on GitHub](https://github.com/earendil-works/pi)
