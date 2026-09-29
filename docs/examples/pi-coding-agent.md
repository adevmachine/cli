# Pi as your coding agent

[Pi](https://pi.dev) is an open-source terminal coding agent. It works like
Claude Code or Codex: you point it at a project folder and it reads, edits
and runs code for you, switching between many AI providers as you like.
This example installs Pi in a workspace so it runs on your server, not your
laptop, and keeps working after you close the terminal.

**You need:** a workspace — see [getting started](../getting-started.md).

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

There's no devmachine package for Pi yet, so you install it the way Pi's own
docs say, inside the workspace's account.

### 1. Open the workspace

```
devmachine ssh alice
```

You land inside tmux. Anything you start here keeps running after you close
the terminal.

### 2. Install Pi

The workspace already has Node through the `dev` package, which Pi needs
(version 22.19 or newer). Install Pi with npm:

```
npm install -g --ignore-scripts @earendil-works/pi-coding-agent
```

### 3. Sign in

```
pi
```

The first time you run `pi`, use the `/login` command inside it to connect a
subscription or an API key. Pi supports many providers — Anthropic, OpenAI,
Google, and others — and you can switch models later with `/model`.

### 4. Start working

```
cd ~/dev/my-project
pi
```

Pi reads any `AGENTS.md` in the project for instructions, the same way other
agents do. Close the terminal any time — `devmachine ssh alice` brings you
back to the same tmux session, with Pi still running.

## Teach Pi the devmachine CLI

`devmachine skills add` also teaches Pi how to run devmachine commands, on
your own computer:

```
devmachine skills add --agent pi
```

This helps in a session on your computer where you ask Pi to manage your
machines and workspaces — not inside the workspace where Pi itself runs.

## With your agent

Open a session on your own computer (`devmachine skills add --agent pi`, or
the plain `devmachine skills add`, taught it the CLI) and say:

```text
SSH into my devmachine workspace alice and install the Pi coding agent
there with npm, following pi.dev's own instructions.
```

The agent runs `devmachine ssh alice`, then the npm install command, inside
the workspace's tmux session. Signing in with `/login` is yours to do — that
is your own account.

**Check it:** inside the workspace, run `pi --version`; then start it in a
project folder and ask it a question about the code there.

Source: [pi.dev](https://pi.dev/), [Pi coding agent on GitHub](https://github.com/earendil-works/pi)
