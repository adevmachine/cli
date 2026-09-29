# Express site with TLS

Run an Express app in workspace `alice`, reachable at `https://app.example.com`
with an HTTPS certificate that renews itself.

**You need:** workspace `alice` from [Getting started](../getting-started.md),
and `app.example.com` pointed at your machine (or a DNS provider package
installed, so `expose add` points it for you).

## 1. Add Caddy to the machine

```
devmachine packages add caddy
devmachine sync
```

Caddy answers every exposed site and gets its own certificate. A new machine has no web server: `setup` installs no packages on the
machine itself. Skip this step if Caddy is already there —
`devmachine packages list` shows `caddy` with your machine's name next to it.

## 2. Write and start the app

```
devmachine ssh alice
mkdir app && cd app
npm init -y && npm install express
cat > server.js <<'EOF'
const express = require('express');
const app = express();
app.get('/', (req, res) => res.send('Hello from alice'));
app.listen(3000);
EOF
node server.js
```

Leave it running and close the terminal: the login is inside tmux, so the
app keeps running on the server. To have it start again after a reboot, use
a process manager such as [pm2](https://pm2.keymetrics.io/docs/usage/quick-start/).

## 3. Expose the port

Back on your own computer:

```
devmachine expose add alice 3000 --host app.example.com --publish
devmachine sync
```

`sync` writes the Caddy route and gets the certificate for the host.

## 4. Point the domain, if it was not automatic

If a DNS provider package (`hostinger`, `cloudflare`) is installed, `expose
add` already wrote the record — see [DNS](../concepts/dns.md). Otherwise it
printed the record to create by hand at your registrar.

**Check it:** `curl https://app.example.com` returns "Hello from alice",
with a valid certificate.

Source: [Express — Hello world](https://expressjs.com/en/starter/hello-world.html)
