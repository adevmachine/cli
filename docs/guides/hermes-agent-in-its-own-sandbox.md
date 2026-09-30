---
description: "Run Nous Research's Hermes Agent always on, in a sandbox of its own."
category: Agents
level: Beginner
needs:
  - "A machine from Getting started"
related:
  - openclaw-in-its-own-sandbox.md
  - pi-coding-agent.md
  - agno-agentos-with-control-plane.md
---
# Hermes Agent, in its own sandbox

Run [Hermes Agent](https://github.com/NousResearch/hermes-agent), Nous
Research's open source agent, always on, in a sandbox of its own: a
workspace `hermes`, which is a separate account on your server with its own
files and model keys, no `sudo`, and no way into your other workspaces.

**You need:** nothing beyond [Getting started](../getting-started.md).

## Before you start: machine, skills, workspace

```
curl -fsSL https://mydevmachine.sh/install.sh | sh
devmachine setup
devmachine skills add
devmachine workspaces new hermes
devmachine sync
```

Already have a machine? Skip `setup`. Already have the workspace? Skip the
last two. See [getting started](../getting-started.md) for what each command
does.

## By hand

### 1. Install Hermes Agent

```
devmachine ssh hermes
curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash
```

The installer sets up Python, Node and the other tools Hermes needs itself,
so the workspace does not need them added first.

### 2. Add a model

```
hermes setup --portal
```

This walks through picking a model. It supports the Nous Portal (one login
covers a model plus its tool set), or your own OpenAI or OpenRouter key —
`hermes model` switches later, and `hermes config set` stores a key by
hand. Whichever you pick, the choice and the key stay in this workspace's
own account, never in your configuration.

### 3. Start it, and keep it running

```
hermes
```

You do not need tmux yourself: every SSH login to a workspace already lands
in a tmux session called `main` (the `zsh` package does this), and it stays
up after you log out. Detach with `Ctrl-b d`, or just close the terminal —
`devmachine ssh hermes` puts you back in the same session.

For messaging apps (Telegram, Discord, Slack, WhatsApp, Signal) instead of
the terminal, run `hermes gateway setup` then `hermes gateway start`. To
make Hermes survive a reboot too, not just a closed terminal, see the
project's own docs on daemon and background backends.

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Create a devmachine workspace called hermes and install Hermes Agent
in it, following the project's own install script.
```

The agent creates the workspace, SSHes in, and runs the installer. Picking a
model and running `hermes setup --portal` (or handing over an API key) is
yours to do — a login or a key is not something the agent can do for you.

**Check it:** `devmachine ssh hermes` again, then `hermes` — it comes back
to a working session with the model you configured.

Source: [Hermes Agent](https://hermes-agent.nousresearch.com/),
[Hermes Agent on GitHub](https://github.com/NousResearch/hermes-agent)
