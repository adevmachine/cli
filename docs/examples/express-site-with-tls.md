# Express site with TLS

Run an Express app in a workspace and put it on the internet at a real
domain, with HTTPS that renews itself. This is the plain Node case: one
process, one port, no container.

**You need:** a Debian or Ubuntu VPS, and a domain such as `app.example.com`
pointed at it (or a DNS provider package installed, so `expose add` points it
for you — see [DNS](../concepts/dns.md)).

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

### 1. Add Caddy to the machine

```
devmachine packages add caddy
devmachine sync
```

Caddy answers every exposed site and gets its own certificate. A new machine
has no web server, so this step is needed once per machine — skip it if
`devmachine packages list` already shows `caddy` there.

### 2. Write and start the app

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

Leave this running and close the terminal: the login is inside tmux, so the
app keeps going. To make it survive a reboot too, run it under a process
manager such as [pm2](https://pm2.keymetrics.io/docs/usage/quick-start/).

### 3. Expose the port

Back on your own computer:

```
devmachine expose add alice 3000 --host app.example.com --publish
devmachine sync
```

`sync` writes the Caddy route and gets the certificate for the host. If no
DNS provider package is installed, `expose add` prints the record to create
by hand at your registrar instead of writing it for you.

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Set up an Express "hello world" app in my devmachine workspace alice,
listening on port 3000, and expose it at app.example.com.
```

The agent runs the same commands: adds `caddy` if it is missing, writes
`server.js` over SSH, starts it, then runs `expose add` and `sync`. You still
approve `sync` when it asks, and if `setup` has not run yet, you check the
fingerprint yourself — the agent cannot do that part.

**Check it:** `curl https://app.example.com` returns "Hello from alice",
with a valid certificate.

Source: [Express — Hello world](https://expressjs.com/en/starter/hello-world.html)
