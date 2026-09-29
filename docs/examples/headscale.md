# Your own Tailscale with Headscale

**Not yet tested end to end on a devmachine.** The steps below follow
Headscale's own docs and devmachine's `tailscale` package as written; if
something does not match what you see, trust what is in front of you over
this page.

[Headscale](https://headscale.net) is an open source, self-hosted
replacement for Tailscale's control server. You get the same private
network, the same `tailscale` client on every device, but you run the
server that coordinates it — no third party in the loop. This example puts
Headscale on a small server, joins your devmachine server to it, and joins
your own computer too.

**You need:** a second small Debian or Ubuntu server for Headscale itself
(or reuse your devmachine server — see the note below), and your devmachine
server already reachable over SSH.

## Before you start: machine, skills, workspace

```
brew install mydevmachine/tap/devmachine
devmachine setup
devmachine skills add
devmachine workspaces new acme
devmachine sync
```

Already have a machine? Skip `setup`. Already have the workspace? Skip the
last two. See [getting started](../getting-started.md) for what each command
does. This example does not need a new workspace — it only touches the
machine — but the block above is the standard starting point if you have
neither yet.

## By hand

### 1. Install Headscale

On the server that will run it (a separate small VPS, or your devmachine
server itself — Headscale is just one more service on it), install the
official `.deb` package from the
[Headscale releases page](https://github.com/juanfont/headscale/releases):

```
wget --output-document=headscale.deb \
  https://github.com/juanfont/headscale/releases/download/v<version>/headscale_<version>_linux_amd64.deb
sudo apt install ./headscale.deb
```

Edit `/etc/headscale/config.yaml` to set your server's URL, then start it:

```
sudo systemctl enable --now headscale
```

See [Headscale's own install docs](https://headscale.net/stable/setup/install/official/)
for the config fields and current version.

### 2. Create a user and a pre-auth key

```
headscale users create acme
headscale users list
headscale preauthkeys create --user <id> --expiration 1h
```

`--user` takes the user's number from the `ID` column of `users list`, not
its name. The key printed is what a device trades for a place on your
network. It expires — here, in one hour — and by default works once, so
generate a fresh one per device.

### 3. Join your devmachine server

devmachine's `tailscale` package runs a plain `tailscale up` when you use
`devmachine login tailscale` — it has no flag for a custom control server.
So for Headscale, add the package to get the `tailscale` binary installed,
then run the join yourself as admin, over SSH, with the login server flag:

```
devmachine packages add tailscale
devmachine sync
devmachine ssh
sudo tailscale up --login-server https://your-headscale.example.com --authkey <the key from step 2>
```

`devmachine ssh` with no workspace name opens a session on the machine
itself, as its admin — the right place to run a command devmachine does not
wrap.

### 4. Join your own computer

Install Tailscale from [tailscale.com/download](https://tailscale.com/download),
then join the same Headscale server:

```
tailscale up --login-server https://your-headscale.example.com --authkey <a fresh key>
```

### 5. Use the private address

Once both sides are joined, add the machine to `config.yml` with its
Headscale-assigned address — `tailscale status` on your computer shows it,
typically in the `100.64.0.0/10` range:

```yaml
machines:
  - name: main
    hosts:
      - 100.64.0.5
      - 203.0.113.10
```

devmachine's `tailscale:<name>` shorthand asks the `tailscale` command to
resolve the name, which works the same whether that command is talking to
Tailscale's own service or to your Headscale server — see
[addresses and fallback](../how-it-works/addresses-and-fallback.md). This
page has not confirmed that shorthand against a real Headscale server, so
the plain address above is the safer first step; try `tailscale:main` once
you have confirmed it resolves.

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
My devmachine server needs to join my Headscale server at
https://your-headscale.example.com. Add the tailscale package, then join
it as admin using the login server, not a normal Tailscale login.
```

The agent runs `packages add tailscale` and `sync`, then tells you it needs
a pre-auth key from your Headscale server — generating one is your call,
since it decides who gets on your network. Once you paste a key, the agent
runs `tailscale up --login-server ... --authkey ...` over `devmachine ssh`.
Joining your own computer to Headscale is yours to do: it needs the
Tailscale app installed and signed in on your machine, not the server's.

**Check it:** `tailscale status`, run on your computer, lists your
devmachine server with a `100.64.x.x` address and shows it online.

Source: [Headscale](https://headscale.net),
[Headscale — Official releases](https://headscale.net/stable/setup/install/official/),
[Headscale — Getting started](https://headscale.net/stable/usage/getting-started/)
