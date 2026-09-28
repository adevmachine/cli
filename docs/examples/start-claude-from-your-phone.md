# Start Claude from your phone

Keep a Claude Code Remote Control session up on workspace `alice`, so you can
open it from the phone without ever opening an SSH session yourself.

**You need:** workspace `alice`, with `claude-code` installed and logged in —
see [Claude Code, controlled from your phone](claude-code-remote-control.md).

## What `claude-remote-control` does

It installs a systemd user unit, plus `loginctl enable-linger`, so the
account's systemd keeps running with nobody logged in. The unit runs `claude
remote-control`, which starts in server mode and pre-creates one session in
its working directory — the same session a person would get by running that
command by hand. `Restart=always` brings it back after a crash or a network
drop. It only starts once the account has logged in to `claude`; with no
login yet, `sync` says so instead of looping.

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
