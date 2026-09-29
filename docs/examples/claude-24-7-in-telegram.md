# Claude 24/7 in Telegram

Claude Code can read messages from a Telegram bot and reply in the same
chat, while it keeps working on your server. You text it from your phone,
it does the work against your real files, and the answer comes back in
Telegram — no need to open a terminal.

This uses Claude Code's **channels** feature, which pushes messages into a
session that is already running. Anthropic marks channels a **research
preview**: the `--channels` flag isn't even listed in `claude --help` yet,
and its exact form may still change.

**You need:** workspace `alice` with `claude-code` installed — see
[getting started](../getting-started.md). A Claude Pro, Max, Team, or
Enterprise plan, or a Console API key (channels don't work on Bedrock,
Google's Agent Platform, or Microsoft Foundry). If your Claude account
belongs to a Team or Enterprise organization, an Owner must turn channels on
first, under **claude.ai → Admin settings → Claude Code → Channels**. A
Telegram account, to create the bot.

## Before you start: machine, skills, workspace

```
brew install mydevmachine/tap/devmachine
devmachine setup
devmachine skills add
devmachine workspaces new alice
devmachine sync
```

Already have a machine? Skip `setup`. Already have the workspace? Skip the
last two. See [getting started](../getting-started.md) for what each command
does.

## By hand

### 1. Add Claude Code

```
devmachine packages add claude-code --workspace alice
devmachine sync
```

### 2. Sign in, once

```
devmachine login claude --workspace alice
```

This opens a real terminal so you can finish the sign-in yourself — see
[credentials](../concepts/credentials.md).

### 3. Open the workspace and start Claude Code

```
devmachine ssh alice
claude
```

You land inside tmux, so Claude Code keeps running after you close the
terminal.

### 4. Create the Telegram bot

In Telegram, open [BotFather](https://t.me/BotFather) and send `/newbot`.
Give it a name and a username ending in `bot`. BotFather gives you a token —
copy it, and never paste it anywhere but the step below.

### 5. Install and configure the Telegram plugin

Inside the running Claude Code session:

```
/plugin install telegram@claude-plugins-official
```

If it says the marketplace isn't found, add it first:

```
/plugin marketplace add anthropics/claude-plugins-official
```

Choose the user scope when asked, so the plugin works in every session.
Then set the token you got from BotFather:

```
/telegram:configure <token>
```

This saves the token to `~/.claude/channels/telegram/.env` on the server —
it never appears on this page or in your devmachine configuration.

### 6. Restart with the channel on

```
claude --channels plugin:telegram@claude-plugins-official
```

This starts the Telegram plugin, which begins checking for messages from
your bot. The Telegram plugin is a Bun script; the workspace already has
Bun from the `dev` package.

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
channel. Every workspace login already opens inside tmux (see
[keep your sessions running](keep-sessions-running.md)), so once you close
your laptop, `claude --channels ...` and the Telegram plugin both keep
running on the server. `devmachine ssh alice` brings you back to the same
session, still connected.

### When Claude needs your permission

If Claude stops to ask for permission while you are away from the terminal,
the session waits until somebody answers, and your Telegram messages wait with
it. Give the session the permissions its work needs up front (see Claude
Code's [permission modes](https://code.claude.com/docs/en/permission-modes)).
`--dangerously-skip-permissions` skips most prompts, but only use it where
you trust everything the session can reach: a workspace is a good place for
it, because it cannot touch your other workspaces.

## With your agent

Open a session on your own computer and say:

```text
Add Claude Code to my devmachine workspace alice, sign me in, and start a
session there so I can set up the Telegram channel.
```

The agent runs `packages add claude-code --workspace alice` and `sync`,
hands you `devmachine login claude --workspace alice` to sign in yourself,
then opens `devmachine ssh alice` and starts `claude`. From there, creating
the bot in BotFather, running `/plugin install`, `/telegram:configure` with
your token, and pairing your account with `/telegram:access pair` are all
yours to do — the token and the pairing decision belong to you, not the
agent.

**Check it:** send a message to your bot from Telegram; it shows up in the
terminal on the server as an inbound line, and Claude's reply appears back
in the Telegram chat.

Source: [Claude Code — Push events into a running session with channels](https://code.claude.com/docs/en/channels)
