---
description: "Install the Cline CLI in a workspace and connect it to your Cline account or your own API key."
category: Agents
level: Beginner
needs:
  - "A workspace"
related:
  - opencode.md
  - pi-coding-agent.md
  - keep-sessions-running.md
---
# Cline as your coding agent

[Cline](https://cline.bot/cli) is an open-source coding agent. Its CLI
brings the same agent to the terminal. This guide installs it in a
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
devmachine packages add cline --workspace acme
devmachine sync
```

Cline ships only through npm, so the package needs Node. `packages add`
adds the `dev` package too, which brings it. `sync` installs `cline` into
the workspace's own `~/.local`, so a Node upgrade does not remove it.

### 2. Sign in, once

```
devmachine login cline --workspace acme
```

This runs `cline auth` in a real terminal. Pick your Cline account, a
ChatGPT subscription, or your own provider's API key.

### 3. Start working

```
devmachine ssh acme
cd ~/dev/my-project
cline
```

You land inside tmux, so `cline` keeps running when you close the terminal.

## Skills

Cline reads its skills from `~/.cline/skills`, not from `~/.agents/skills`.
Devmachine links each skill there for you: in a workspace with this
package, and on your computer with `devmachine skills add --agent cline`.
See [agent skills](../concepts/agent-skills.md).

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Install the Cline CLI in my devmachine workspace acme.
```

The agent adds the `cline` package and runs `sync`. Signing in stays yours:
`devmachine login cline --workspace acme` opens a real terminal for you to
finish it.

**Check it:** `devmachine run --workspace acme -- cline --version` prints a
version. Then start `cline` in a project folder and ask it about the code.

Source: [Cline CLI](https://cline.bot/cli),
[Cline CLI reference](https://docs.cline.bot/cli/cli-reference)
