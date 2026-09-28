# An agent in its own workspace

Run [OpenClaw](https://openclaw.ai/), an autonomous agent, in a dedicated
workspace `agent` — its own account, files and logins, isolated from your own
work.

**You need:** nothing beyond [Getting started](../getting-started.md).

## 1. Create the workspace

```
devmachine workspaces new agent
devmachine sync
```

A workspace is one Linux account: whatever the agent installs or breaks stays
inside it, and never touches `alice` or any other workspace — see
[machines and workspaces](../concepts/machines-and-workspaces.md).

## 2. Install OpenClaw

```
devmachine ssh agent
curl -fsSL https://openclaw.ai/install.sh | bash
```

The installer sets up Node itself if the account has none.

## 3. Keep it running

```
tmux new -s agent
openclaw onboard
```

`openclaw onboard` walks through first-time setup and then runs the Gateway
in the foreground; tmux keeps that session alive after you log out. Detach
with `Ctrl-b d`. OpenClaw's own docs also cover installing it as a systemd
user service (`openclaw onboard --install-daemon`), for a machine you want it
to survive a reboot on too.

**Check it:** back in the workspace (`devmachine ssh agent`, then `tmux
attach -t agent`), the Gateway is still running.

Source: [OpenClaw — Install](https://docs.openclaw.ai/install)
