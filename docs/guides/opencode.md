---
description: "Install opencode in a workspace, try it on its free models, then connect your own provider."
category: Agents
level: Beginner
needs:
  - "A workspace"
related:
  - pi-coding-agent.md
  - antigravity-cli.md
  - keep-sessions-running.md
---
# opencode as your coding agent

[opencode](https://opencode.ai) is an open-source terminal coding agent
that works with many model providers. It also ships a few free models, so
it answers before you sign in to anything. This guide installs it in a
workspace, so it runs on your server and keeps working after you close the
terminal.

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
devmachine packages add opencode --workspace acme
devmachine sync
```

`sync` runs opencode's own installer inside the workspace's account. It puts
`opencode` in `~/.opencode/bin` and that folder on the PATH.

### 2. Try it

```
devmachine ssh acme
cd ~/dev/my-project
opencode
```

With no login, opencode uses its own free models. You land inside tmux, so
it keeps running when you close the terminal.

### 3. Connect your own provider, once

```
devmachine login opencode --workspace acme
```

This runs `opencode auth login` in a real terminal: pick a provider, then
paste an API key or follow its sign-in. The keys stay in this workspace's
`~/.local/share/opencode/auth.json`.

## Skills

opencode reads `~/.agents/skills`, so every skill Devmachine installs there
works without another step. See [agent skills](../concepts/agent-skills.md).

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Install opencode in my devmachine workspace acme.
```

The agent adds the `opencode` package and runs `sync`. Connecting a provider
stays yours: `devmachine login opencode --workspace acme` opens a real
terminal for you to finish it.

**Check it:** `devmachine run --workspace acme -- opencode --version` prints
a version. Then start `opencode` in a project folder and ask it about the
code.

Source: [opencode docs](https://opencode.ai/docs/),
[opencode CLI](https://opencode.ai/docs/cli/)
