# Start Claude from your phone

Keep a Claude Code Remote Control session up on workspace `alice`, so you can
open it from the phone without ever opening an SSH session yourself.

**You need:** workspace `alice`, with `claude-code` installed and logged in —
see [Claude Code, controlled from your phone](claude-code-remote-control.md).

## What `claude-remote-control` does

It runs `claude remote-control` in the background on the server, and starts
it again on its own after a crash, a network drop, or a reboot — so a session
is always waiting for you, even with nobody logged in. It only starts once
the account has signed in to `claude`; with no sign-in yet, `sync` tells you
that instead of trying anyway.

## 1. Add the package

```
devmachine packages add claude-remote-control --workspace alice
devmachine sync
```

## 2. Point it at a project, if you want one specific directory

```
devmachine workspaces edit alice --set claude-remote-control.working_directory=/home/alice/app
devmachine sync
```

Empty keeps the default, `~/dev`.

## 3. Open it from the phone

In the Claude app, tap **Code**, and find the session named `alice` (or the
`session_name` you set) in the list — no SSH needed.

**Check it:** the session shows a green status dot, online, and a message
typed there reaches the machine.

Source: [Claude Code — Remote Control](https://code.claude.com/docs/en/remote-control)
