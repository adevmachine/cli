---
description: "Install Moonshot AI's Kimi Code CLI in a workspace and sign in with a device code."
category: Agents
level: Beginner
needs:
  - "A workspace"
  - "A Kimi account"
related:
  - opencode.md
  - antigravity-cli.md
  - keep-sessions-running.md
---
# Kimi Code as your coding agent

[Kimi Code](https://www.kimi.com/code/docs/en/) (`kimi`) is Moonshot AI's
terminal coding agent, built for the Kimi models. This guide installs it in
a workspace, so it runs on your server and keeps working after you close
the terminal.

**You need:** a workspace — see [getting started](../getting-started.md) —
and a Kimi account.

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
devmachine packages add kimi-code --workspace acme
devmachine sync
```

`sync` runs Kimi's own installer inside the workspace's account. It puts
`kimi` in `~/.kimi-code/bin` and that folder on the PATH.

### 2. Sign in, once

```
devmachine login kimi --workspace acme
```

This runs `kimi login` in a real terminal. It prints a link and a short
code: open the link on any device, enter the code, and approve. The
terminal waits until you do. An account on kimi.ai rather than kimi.com
signs in from the workspace with `kimi login --region global`.

### 3. Start working

```
devmachine ssh acme
cd ~/dev/my-project
kimi
```

You land inside tmux, so `kimi` keeps running when you close the terminal.

## Skills

Kimi Code reads `~/.agents/skills`, so every skill Devmachine installs there
works without another step. On your computer,
`devmachine skills add --agent kimi` does the same. See
[agent skills](../concepts/agent-skills.md).

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Install Kimi Code in my devmachine workspace acme.
```

The agent adds the `kimi-code` package and runs `sync`. Signing in stays
yours: `devmachine login kimi --workspace acme` opens a real terminal for
you to approve the code.

**Check it:** `devmachine run --workspace acme -- kimi --version` prints a
version. Then start `kimi` in a project folder and ask it about the code.

Source: [Kimi Code CLI](https://www.kimi.com/code/docs/en/),
[Kimi Code on GitHub](https://github.com/MoonshotAI/kimi-code)
