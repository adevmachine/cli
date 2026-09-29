# Start Claude from your phone

Keep a Claude Code Remote Control session up on a workspace, so you can open
it from the phone at any time — no SSH session to start first, no terminal
left open on your computer.

**You need:** [Claude Code, controlled from your phone](claude-code-remote-control.md)
done first: a workspace with `claude-code` installed and `alice` already
signed in with `devmachine login claude --workspace alice`.

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

## What `claude-remote-control` does

It runs `claude remote-control` in the background on the server, and starts
it again on its own after a crash, a network drop, or a reboot — so a
session is always waiting for you, even with nobody logged in. It only
starts once the account has signed in to `claude`; with no sign-in yet,
`sync` tells you that instead of trying anyway.

## By hand

### 1. Add the package

```
devmachine packages add claude-remote-control --workspace alice
devmachine sync
```

### 2. Point it at a project, if you want one specific directory

```
devmachine workspaces edit alice --set claude-remote-control.working_directory=/home/alice/app
devmachine sync
```

Empty keeps the default, `~/dev`.

### 3. Open it from the phone

In the Claude app, tap **Code**, and find the session named `alice` (or the
`session_name` you set) in the list — no SSH needed.

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Add claude-remote-control to my devmachine workspace alice, working
directory /home/alice/app.
```

The agent adds the package, sets `working_directory`, and runs `sync`. If
`alice` has not signed in to `claude` yet, `sync` says so — that sign-in
step (`devmachine login claude --workspace alice`) is yours to do, since it
opens a real terminal for you.

**Check it:** the session shows a green status dot, online, and a message
typed there reaches the machine.

Source: [Claude Code — Remote Control](https://code.claude.com/docs/en/remote-control)
