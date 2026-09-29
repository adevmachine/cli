# A Claude that never sleeps

Start Claude Code on a workspace, turn on Remote Control with one command
typed inside the session, and open the same conversation from the Claude app
on your phone or from Claude Desktop — while it keeps running on the server,
inside tmux, even after you close your laptop.

This example uses nothing but the `claude-code` package and the `/rc`
command Claude Code already ships with. For a version that reconnects itself
after a crash or a reboot with no command to type, see the note at the
bottom.

**You need:** workspace `acme` with `claude-code` installed — see
[Getting started](../getting-started.md). A Claude Pro, Max, Team, or
Enterprise plan; Remote Control does not work with an API key.

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

### 1. Add Claude Code

```
devmachine packages add claude-code --workspace acme
devmachine sync
```

### 2. Sign in, once

```
devmachine login claude --workspace acme
```

This opens a real terminal so you can finish the sign-in yourself — see
[credentials](../concepts/credentials.md). Remote Control needs a real
claude.ai sign-in, not an API key.

### 3. Start a session

```
devmachine ssh acme
claude
```

`ssh` lands you inside the workspace's tmux session before Claude Code even
starts.

### 4. Turn on Remote Control

Inside the running session, type:

```
/rc
```

Claude Code shows a one-time confirmation the first time you do this; accept
it. It then prints a session URL and a QR code, and the terminal shows an
`/rc active` indicator.

### 5. Open it from another device

Scan the QR code with the Claude app, or open [claude.ai/code](https://claude.ai/code)
and find the session by name — a connected session shows a computer icon
with a green status dot. From there you can read the conversation, send
messages, and see what Claude is doing, from your phone or from Claude
Desktop.

## Why closing the laptop does not stop it

Remote Control itself only keeps a session alive while the local `claude`
process keeps running — closing the terminal that started it normally takes
the session offline with it. The official docs say the fix is to run it
inside `tmux` or `screen` on the remote machine. On devmachine, every
workspace login already opens inside tmux (see
[keep your sessions running](keep-sessions-running.md)), so this is already
true for you: close your laptop, and the `claude` process, and the Remote
Control session with it, keep running on the server.

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Add Claude Code to my devmachine workspace acme, sign me in, and start a
session there so I can turn on Remote Control.
```

The agent runs `packages add claude-code --workspace acme` and `sync`,
hands you `devmachine login claude --workspace acme` to sign in yourself,
then opens `devmachine ssh acme` and starts `claude`. Typing `/rc` and
accepting the confirmation is yours to do — that decision is not one an
agent should make for you.

**Check it:** send a message from the Claude app on your phone; it shows up
in the terminal on the server, and a reply typed there shows up on the
phone.

## Want this without typing `/rc` yourself?

The `claude-remote-control` package does the same thing automatically —
starting the session on its own, and starting it again after a crash,
network drop, or reboot, with nobody logged in. See
[start Claude from your phone](start-claude-from-your-phone.md).

Source: [Claude Code — Remote Control](https://code.claude.com/docs/en/remote-control)
