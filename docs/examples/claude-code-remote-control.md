# Claude Code, controlled from your phone

Run Claude Code in workspace `alice` and open a session from the Claude
iPhone or Android app.

**You need:** workspace `alice` from [Getting started](../getting-started.md).

## 1. Add Claude Code, with Remote Control on by default

```
devmachine packages add claude-code --workspace alice
devmachine workspaces edit alice --set claude-code.remote_control_at_startup=true
devmachine sync
```

`remote_control_at_startup` connects every session to Remote Control as it
opens, instead of waiting for somebody to type `/rc`.

## 2. Log in as alice, once

```
devmachine login alice/claude
```

This opens a real terminal, because a device login reaches a person or it
reaches nobody — see [credentials](../concepts/credentials.md).

## 3. Start a session

```
devmachine ssh alice
claude
```

With `remote_control_at_startup` on, Claude Code prints a session URL and a
QR code as the session opens. Without it, run `/rc` inside the session to get
the same thing.

## 4. Open it from the phone

Install the Claude app ([iOS](https://apps.apple.com/us/app/claude-by-anthropic/id6473753684),
[Android](https://play.google.com/store/apps/details?id=com.anthropic.claude)),
scan the QR code, or find the session by name under **Code** in the app.

**Check it:** a message sent from the phone appears in the terminal, and a
reply typed in the terminal appears on the phone.

For a session that is available from the phone without an SSH session open at
all, see [start Claude from your phone](start-claude-from-your-phone.md).

Source: [Claude Code — Remote Control](https://code.claude.com/docs/en/remote-control)
