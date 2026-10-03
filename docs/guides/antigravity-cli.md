---
description: "Install Google's Antigravity CLI in a workspace and sign in once, from your own terminal."
category: Agents
level: Beginner
needs:
  - "A workspace"
  - "A Google account, or a Gemini API key"
related:
  - opencode.md
  - kimi-code.md
  - keep-sessions-running.md
---
# Antigravity CLI as your coding agent

[Antigravity CLI](https://antigravity.google/docs/cli/overview/) (`agy`) is
Google's terminal coding agent. Point it at a project folder and it reads,
edits and runs code for you, with Gemini models. This guide installs it in a
workspace, so it runs on your server and keeps working after you close the
terminal.

**You need:** a workspace — see [getting started](../getting-started.md) —
and a Google account or a Gemini API key.

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
devmachine packages add antigravity --workspace acme
devmachine sync
```

`sync` runs Google's own installer inside the workspace's account. It puts
`agy` in `~/.local/bin`, and from then on `agy` updates itself.

### 2. Sign in, once

```
devmachine login antigravity --workspace acme
```

This starts `agy` in a real terminal. Over SSH it does not open a browser:
it prints a link. Open the link on your computer, sign in with Google, and
paste the code back into the terminal. See
[why a login cannot be automated](../how-it-works/why-a-login-cannot-be-automated.md).

With a Gemini API key instead, set `"modelProvider": "gemini"` in
`~/.gemini/antigravity-cli/settings.json` and export the key as
`GEMINI_API_KEY` in the workspace. `agy` reads it only from the
environment.

### 3. Start working

```
devmachine ssh acme
cd ~/dev/my-project
agy
```

You land inside tmux, so `agy` keeps running when you close the terminal.
`devmachine ssh acme` brings you back to the same session.

## Skills

Antigravity CLI reads its skills from `~/.gemini/antigravity-cli/skills`,
not from `~/.agents/skills`. Devmachine links each skill there for you: in a
workspace with this package, and on your computer with
`devmachine skills add --agent antigravity`. See
[agent skills](../concepts/agent-skills.md).

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Install the Antigravity CLI in my devmachine workspace acme.
```

The agent adds the `antigravity` package and runs `sync`. Signing in stays
yours: `devmachine login antigravity --workspace acme` opens a real
terminal for you to finish it.

**Check it:** `devmachine run --workspace acme -- agy --version` prints a
version. Then start `agy` in a project folder and ask it about the code.

Source: [Antigravity CLI](https://antigravity.google/docs/cli/overview/),
[installation and auth](https://antigravity.google/docs/cli/install/)
