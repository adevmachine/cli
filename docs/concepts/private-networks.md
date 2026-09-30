# Tailscale and other private networks

A private network gives your server a second address that only your own
devices can reach. It is not a replacement for the public address `setup`
gives you — it is a backup that keeps working when the public one has
trouble, and a way to reach things you never want on the internet at all.

## Why bother

- **You can still get in when public SSH is blocked.** A hotel, a work
  network, an airport — some of them block port 22 outright. A private
  address does not go through that path.
- **mosh works properly.** mosh needs a UDP range open to the internet to
  work on the public address, which most people do not want. Over a private
  network, that range never has to face the public internet at all.
- **Private apps stay private.** A database viewer, an internal dashboard,
  a dev server — reachable by you, without `devmachine expose` and without a
  DNS record anyone could stumble on.

See [reaching your server](reaching-your-server.md) for the public address
this builds on, and
[addresses and fallback](../how-it-works/addresses-and-fallback.md) for how
devmachine picks between several addresses.

## Tailscale, step by step

[Tailscale](https://tailscale.com) is supported through the `tailscale`
package. The CLI itself knows no network: the package tells it how to turn
`tailscale:<name>` into an address, how to join, and what the machine is
called — see [the network package contract](../reference/network-package-contract.md).

`setup` asks about Tailscale right after the essentials, and adding the
package is all "yes" does — the sign-in and the private address still need
the two steps below.

1. Add the package and sync, if `setup` did not already:

   ```
   devmachine packages add tailscale
   devmachine sync
   ```

2. Sign the server in to your Tailscale account:

   ```
   devmachine login tailscale
   ```

   This runs the package's `join` on the server, in a real terminal — it
   calls `tailscale up`, and you finish the sign-in in your browser. Once it
   succeeds, devmachine asks the server for its own name on the tailnet and
   adds it to `config.yml` for you, above the public address:

   ```yaml
   machines:
     - name: main
       hosts:
         - tailscale:main
         - 203.0.113.10
   ```

   The public address stays as a fallback. When the name cannot be read —
   the package failed to install, or something else went wrong — devmachine
   prints this same block for your machine instead, with `- tailscale:<name>`
   where the new line goes. `<name>` is what `tailscale status` on the server
   lists for it.

3. Install Tailscale on your own computer, from
   [tailscale.com/download](https://tailscale.com/download), and sign in to
   the same account.

devmachine tries `tailscale:main` first and falls back to the public address
if Tailscale is not running on your computer — so turning Tailscale off
never locks you out. `devmachine resolve` shows which address it will use
and why it skipped one.

### Your own control server

Set `tailscale.login_server` to your Headscale server's URL before
`devmachine login tailscale`, and the join uses it:

```yaml
machines:
  - name: main
    settings:
      tailscale.login_server: https://net.example.com
```

See [Your own Tailscale with Headscale](../guides/headscale.md).

### Sending your traffic through the server

Set the machine as an exit node, so your own internet traffic can route
through it:

```yaml
machines:
  - name: main
    settings:
      tailscale.exit_node: true
```

```
devmachine sync
```

The package's `join` also advertises the exit node when this is on, the next
time you run `devmachine login tailscale`. An exit node also needs approving
once in the
[Tailscale admin console](https://login.tailscale.com/admin/machines) —
Tailscale will not route traffic through a machine nobody approved for it,
even if it is running the setting.

## Closing public SSH — what devmachine can and can't do

Once the private address works, it is tempting to close public SSH
altogether. devmachine's `firewall` package always allows OpenSSH — there is
no setting in it to turn that off. If you want public SSH closed, that is a
`ufw` change you make yourself on the server, outside what devmachine
manages, and only after you have confirmed the private address gets you in
every time.

## Comparing your options

| | What it is | Good for | Watch out for |
| --- | --- | --- | --- |
| **Tailscale** | Hosted, WireGuard-based mesh network | Fastest to set up, works almost anywhere, devmachine resolves `tailscale:<name>` for you | Your traffic's control plane is Tailscale's servers (the data itself is peer-to-peer) |
| **Headscale** | Self-hosted, open source Tailscale control server | Full control, no third party in the loop | You run and maintain the control server yourself — see [Your own Tailscale with Headscale](../guides/headscale.md) |
| **Plain WireGuard** | The protocol Tailscale is built on, configured by hand | No account, no control server, total control | You manage keys and routing yourself — no automatic discovery |
| **ZeroTier** | Another hosted mesh network, similar shape to Tailscale | An alternative if you already use it | No package for it yet — add the address to `hosts:` directly, or write a network package |
| **Provider private network** | A VPC or private network your VPS provider offers | Often free, no extra software | Usually only reaches other servers from the same provider, not your laptop |

Any of these works with devmachine the same way: once your server has an
address your computer can reach, add it to `hosts:` — first in the list, so
it is tried before the public one. Or write a package with a `network:`
block, and `<prefix>:<name>` entries work for it the way `tailscale:` does.

Source: [Tailscale — Download](https://tailscale.com/download),
[Tailscale — Exit nodes](https://tailscale.com/kb/1103/exit-nodes)
