# Docker site on 8080

Run a site in a Docker container and put it on the internet at a real
domain, with HTTPS that renews itself. This is the container case: the app
only listens on `localhost`, and Caddy is the only thing the internet talks
to.

**You need:** a Debian or Ubuntu VPS, and a domain such as
`site.example.com` pointed at it (or a DNS provider package installed, so
`expose add` points it for you — see [DNS](../concepts/dns.md)).

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

### 1. Add Docker and Caddy to the machine

```
devmachine packages add docker
devmachine packages add caddy
devmachine sync
```

A new machine has no web server or Docker: `setup` installs no packages on
the machine itself. Skip what is already there — `devmachine packages list`
shows each package with your machine's name next to it.

### 2. Put the workspace in the docker group

```
devmachine workspaces edit alice --set workspace.groups=[docker]
devmachine sync
```

A workspace in the `docker` group can do almost anything on the server, so
this is a deliberate step, not the default —
[workspaces](../reference/commands.md#workspaces) says why.

### 3. Run the container

```
devmachine ssh alice
docker run -d --name site -p 127.0.0.1:8080:80 nginx:alpine
```

Publishing on `127.0.0.1` is enough: Caddy reaches the container locally,
and the port itself is never open to the internet.

### 4. Expose it

Back on your own computer:

```
devmachine expose add alice 8080 --host site.example.com --publish
devmachine sync
```

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Run nginx in a Docker container on my devmachine workspace alice,
publishing on 127.0.0.1:8080, and expose it at site.example.com.
```

The agent adds `docker` and `caddy`, puts `alice` in the `docker` group,
starts the container over SSH, then runs `expose add` and `sync`. You still
approve `sync` when it asks, and putting a workspace in the `docker` group
is worth reading before you say yes — it is a lot of trust for one
container.

**Check it:** `curl https://site.example.com` serves the nginx welcome
page, with a valid certificate.

Source: [Docker — `docker run`](https://docs.docker.com/engine/reference/run/),
[nginx image](https://hub.docker.com/_/nginx)
