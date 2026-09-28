# Docker site on 8080

Run a site in a Docker container publishing port 8080, at
`https://site.example.com`.

**You need:** workspace `alice` from [Getting started](../getting-started.md).

## 1. Add Docker and Caddy to the machine

```
devmachine packages add docker
devmachine packages add caddy
devmachine sync
```

## 2. Put the workspace in the docker group

```
devmachine workspaces edit alice --set workspace.groups=[docker]
devmachine sync
```

The `docker` group is effectively root, so a workspace only joins it on
purpose — [workspaces](../reference/commands.md#workspaces) says why.

## 3. Run the container

```
devmachine ssh alice
docker run -d --name site -p 127.0.0.1:8080:80 nginx:alpine
```

Publishing on `127.0.0.1` is enough: Caddy reaches the container locally, and
the port itself is never open to the internet.

## 4. Expose it

Back on your own computer:

```
devmachine expose add alice 8080 --host site.example.com --publish
devmachine sync
```

**Check it:** `curl https://site.example.com` serves the nginx welcome page,
with a valid certificate.

Source: [Docker — `docker run`](https://docs.docker.com/engine/reference/run/),
[nginx image](https://hub.docker.com/_/nginx)
