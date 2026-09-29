# Keep your sessions running

Every SSH login to a devmachine workspace lands inside a tmux session. Start
Claude Code there, close your laptop or lose Wi-Fi, and it is still working
when you reconnect. This example shows it with Claude Code, but it works the
same for any long task.

**You need:** nothing beyond [Getting started](../getting-started.md).

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

### 1. Add Claude Code and sign in

```
devmachine packages add claude-code --workspace alice
devmachine sync
devmachine login claude --workspace alice
```

`login` opens a real terminal so you can finish the sign-in yourself — see
[credentials](../concepts/credentials.md).

### 2. Start a session

```
devmachine ssh alice
claude
```

`ssh` puts you inside the workspace's tmux session, named `main`. `claude`
now runs inside that tmux session, not just inside your SSH connection.

### 3. Give it a long task

Ask it to do something that takes a few minutes — a refactor, a test run,
anything that keeps it working after you stop watching.

### 4. Close the terminal

Close the terminal window, or just let your laptop sleep. The SSH connection
drops, but tmux, and Claude Code inside it, keep running on the server.

### 5. Come back

```
devmachine ssh alice
```

You land back in the same tmux session, with Claude Code still running and
the task further along, or done. On a flaky connection, use
`devmachine mosh alice` instead — mosh survives a dropped or roaming
connection where SSH just breaks.

## tmux basics that matter here

- **Detach without stopping anything:** `Ctrl-b d`. This is what closing the
  terminal does for you automatically, but you can also do it on purpose and
  keep the connection open.
- **List sessions:** `tmux ls`.
- **Reattach by hand:** `tmux attach -t main`, if you ever end up outside the
  session `devmachine ssh` would normally put you in.
- **A second connection gets its own session.** Open a second
  `devmachine ssh alice` while the first is still attached, and you get a new
  tmux session, not a shared view of the first one — so two terminals never
  fight over the same screen.

A reboot stops everything, tmux included. Turn this behavior off with the
[`zsh.tmux_auto_attach`](../reference/settings.md) setting if you would
rather land in a plain shell.

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Add Claude Code to my devmachine workspace alice, sign me in, and start a
session there.
```

The agent runs `packages add claude-code --workspace alice` and `sync`, hands
you `devmachine login claude --workspace alice` to sign in yourself, then
opens `devmachine ssh alice` and starts `claude`. From there, you close the
terminal and come back the same way as above.

**Check it:** close your terminal mid-task, wait a minute, then
`devmachine ssh alice` — the same tmux session is there, and the task has
kept moving.

Source: [Claude Code](https://code.claude.com),
[tmux](https://github.com/tmux/tmux/wiki)
