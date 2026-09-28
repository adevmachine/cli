# Reaching your server

After `setup`, devmachine reaches your server at its public address, with a
key only you have. That is enough to start. This page covers two ways to do
more: a private network, so your server is reachable even when the public
address is not, and private access to apps you do not want on the internet.

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

1. Add Tailscale to the server:

   ```
   devmachine packages add tailscale
   devmachine sync
   ```

2. Sign the server in to your Tailscale account. This opens Tailscale's own
   sign-in on the server; finish it in your browser:

   ```
   devmachine login tailscale
   ```

3. Install Tailscale on your computer and sign in to the same account.

4. Tell devmachine to try the private address first. In `config.yml`, add
   one line above the public address, with the server's name in your tailnet
   (`tailscale status` on your computer lists it):

   ```yaml
   machines:
     - name: main
       hosts:
         - tailscale:main
         - 203.0.113.10
   ```

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
devmachine tunnel alice 3000
```

The app answers at `localhost:3000` on your computer until you press
Ctrl-C. Nothing is published: no DNS record, no open port. See
[publishing](publishing.md) for when to use `expose` instead.
