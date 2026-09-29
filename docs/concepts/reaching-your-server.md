# Reaching your server

After `setup`, devmachine reaches your server at its public address, with a
key only you have. That is enough to start. This page covers three ways to do
more: SSH aliases, so a workspace is reachable by name from any terminal or
editor; a private network, so your server is reachable even when the public
address is not; and private access to apps you do not want on the internet.

## SSH aliases

`devmachine ssh acme` reaches a workspace, but only through this CLI. Tools
that dial `ssh` themselves — a plain `ssh acme-devmachine`, `mosh
acme-devmachine`, VS Code Remote-SSH, Zed, the macOS app — need a Host entry
in `~/.ssh/config`.

`setup` and `machines add` ask once whether to write and maintain those
entries. Say yes, and the CLI keeps a marked block in `~/.ssh/config` up to
date automatically, every time a workspace or a machine changes: after
`workspaces new`, `rm`, `destroy`, `edit --machine`, `machines add`, `rm`,
and at the end of `sync`. Everything outside the block, written by hand or by
something else, is left alone.

Said no at the time, or set up before this existed? Turn it on:

```
devmachine aliases --write
```

`devmachine doctor` and `devmachine machine doctor` both warn when the block
is missing or stale, with the same fix.

## The public address

This is what `setup` gives you. Password logins are off, so only your key gets
in. Add the `firewall` and `fail2ban` packages to close every port you do not
use and to block addresses that keep guessing:

```
devmachine packages add firewall
devmachine packages add fail2ban
devmachine sync
```

## A private network with Tailscale

[Tailscale](https://tailscale.com) puts your computer and your server on a
private network of your own, called a tailnet. Your server gets a private
address that only your devices can reach. It keeps working when the public
address has trouble, and on a network that blocks SSH.

`setup` offers this too, right after it asks about the essentials: say yes,
and it adds the `tailscale` package for you — the same as step 1 below.

1. Add Tailscale to the server, if `setup` did not already:

   ```
   devmachine packages add tailscale
   devmachine sync
   ```

2. Sign the server in to your Tailscale account. This opens Tailscale's own
   sign-in on the server; finish it in your browser:

   ```
   devmachine login tailscale
   ```

   Once you are signed in, devmachine asks the server for its name on the
   tailnet and adds it to `config.yml` for you — one line above the public
   address:

   ```yaml
   machines:
     - name: main
       hosts:
         - tailscale:main
         - 203.0.113.10
   ```

   The public address stays as a fallback. If the name cannot be read (the
   package is not installed yet, or something else went wrong), devmachine
   prints the exact line to add by hand instead.

3. Install Tailscale on your computer and sign in to the same account.

devmachine tries the addresses in order and uses the first that answers. If
Tailscale is off on your computer, it skips that line and uses the public
address, so you are never locked out. See
[addresses and fallback](../how-it-works/addresses-and-fallback.md).

To send your computer's traffic through the server, set
`tailscale.exit_node: true` on the machine and sync.

## Any other private network

devmachine does not need Tailscale. With any VPN that gives your server an
address your computer can reach — WireGuard, ZeroTier, a provider's private
network — add that address to `hosts:` the same way, first in the list.

## Apps you do not want on the internet

A dev server, a database viewer, a mail catcher: these should be reachable
by you, not by anyone who guesses a URL. Open a tunnel instead of publishing
them:

```
devmachine tunnel acme 3000
```

The app answers at `localhost:3000` on your computer until you press
Ctrl-C. Nothing is published: no DNS record, no open port. See
[publishing](publishing.md) for when to use `expose` instead.
