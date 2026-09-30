---
description: "Message Claude Code from Telegram while it runs on your server around the clock."
category: Agents
level: Intermediate
needs:
  - "A workspace with claude-code"
  - "A Claude plan or API key"
  - "A Telegram account"
related:
  - claude-that-never-sleeps.md
  - start-claude-from-your-phone.md
  - keep-sessions-running.md
---
# Claude 24/7 in Telegram

Text your own Claude Code session from Telegram, any time, from your phone.
Claude Code keeps running on your server; you send it a message in a
Telegram chat, it does the work against your real files, and the reply
comes back in the same chat — no terminal to open.

This uses Claude Code's **channels** feature: a running session that reads
messages pushed into it by a plugin. Anthropic marks channels a **research
preview** ([code.claude.com/docs/en/channels](https://code.claude.com/docs/en/channels)):
the `--channels` flag isn't even listed in `claude --help` yet, and its
exact form may still change.

**You need:** workspace `acme` with `claude-code` installed — see
[getting started](../getting-started.md). A Claude account signed in at
[claude.ai](https://claude.ai), on a Pro, Max, Team, or Enterprise plan, or a
Console API key (channels don't work on Bedrock, Google's Agent Platform, or
Microsoft Foundry). If your account belongs to a Team or Enterprise
organization, an Owner must turn channels on first, at
[claude.ai/admin-settings/claude-code](https://claude.ai/admin-settings/claude-code)
(**Channels**). A Telegram account, to create the bot.

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

Two places matter below: **your computer**, where you run `devmachine`
commands, and **inside the workspace**, where Claude Code itself runs, which
you reach with `devmachine ssh acme`.

### 1. Add Claude Code

On your computer:

```
devmachine packages add claude-code --workspace acme
devmachine sync
```

### 2. Sign in, once

On your computer:

```
devmachine login claude --workspace acme
```

This opens a real terminal so you finish the sign-in yourself, against your
[claude.ai](https://claude.ai) account or Console API key — see
[credentials](../concepts/credentials.md).

### 3. Open the workspace and start Claude Code

```
devmachine ssh acme
claude
```

You land inside tmux, so Claude Code keeps running after you close the
terminal — see [keep your sessions running](keep-sessions-running.md).

### 4. Create the Telegram bot

In Telegram, open [BotFather](https://t.me/BotFather) and send `/newbot`.
Give it a display name and a username ending in `bot`. BotFather gives you a
token — copy it, and never paste it anywhere but the step below.

### 5. Install and configure the Telegram plugin

Inside the running Claude Code session (the one from step 3):

```
/plugin install telegram@claude-plugins-official
```

If it says `Marketplace "claude-plugins-official" not found`, add it and
retry:

```
/plugin marketplace add anthropics/claude-plugins-official
```

Choose the user scope when asked, so the plugin works in every session. If
the install summary says `Run /reload-plugins to activate.`, run that too.
Then set the token you got from BotFather:

```
/telegram:configure <token>
```

This saves the token to `~/.claude/channels/telegram/.env`, on the server —
it never appears on this page, in your devmachine configuration, or in the
Telegram chat itself.

### 6. Restart with the channel on

Still inside the workspace:

```
claude --channels plugin:telegram@claude-plugins-official
```

This starts the Telegram plugin, which begins polling for messages from
your bot. The plugin is a [Bun](https://bun.sh) script — the workspace
already has Bun from the `dev` package.

### 7. Pair your Telegram account

Send any message to your bot in Telegram. It replies with a pairing code.
Back in the Claude Code session:

```
/telegram:access pair <code>
```

Then lock the bot down to your account only:

```
/telegram:access policy allowlist
```

Without this, anyone who finds your bot's username could message it.

## Keep it running

The session has to stay open for messages to arrive — closing it stops the
channel. Every workspace login already opens inside tmux, so once you close
your laptop, `claude --channels ...` and the Telegram plugin both keep
running on the server. `devmachine ssh acme` brings you back to the same
session, still connected.

### When Claude needs your permission

If Claude stops to ask for permission while you are away from the terminal,
the session waits until somebody answers, and your Telegram messages wait
with it. Give the session the permissions its work needs up front (see
Claude Code's [permission modes](https://code.claude.com/docs/en/permission-modes)).
`--dangerously-skip-permissions` skips most prompts, but only use it where
you trust everything the session can reach: a workspace is a good place for
it, because it cannot touch your other workspaces.

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Add Claude Code to my devmachine workspace acme, sign me in, and start a
session there so I can set up the Telegram channel.
```

The agent runs `packages add claude-code --workspace acme` and `sync`,
hands you `devmachine login claude --workspace acme` to sign in yourself in
the terminal it opens, then runs `devmachine ssh acme` and starts `claude`.
From there, you stay in the driver's seat for everything Telegram-specific:
create the bot in BotFather yourself, and when the agent runs
`/plugin install telegram@claude-plugins-official`, you paste the bot token
into `/telegram:configure <token>` yourself, inside that same session —
never into the chat with the agent. Pairing your account with
`/telegram:access pair <code>` and setting `/telegram:access policy
allowlist` are also yours: only you know which Telegram account should have
access.

**Check it:** send a message to your bot from Telegram; it shows up in the
terminal on the server as an inbound line, and Claude's reply appears back
in the Telegram chat.

Source: [Claude Code — Push events into a running session with channels](https://code.claude.com/docs/en/channels)
