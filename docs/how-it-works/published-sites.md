# Why a published site lives in the configuration

`expose add` used to write a Caddy file straight onto the machine and
reload Caddy. It worked, but it left the only record of the site on the
machine: rebuild from the configuration, and the sites `expose` had
written never came back.

So the record moved. A route is a field of the workspace that owns it:

    workspaces:
      - name: alice
        routes:
          - {host: app.example.com, port: 8080}

`expose add` writes that line. `sync` renders one file per workspace,
`alice-routes.caddy`, into the `sites.d` folder the `caddy` package
provides, and reloads Caddy when the file changed. `expose rm` deletes
the line, and the next `sync` deletes the block. There is one
direction — configuration to machine — devmachine never reads the machine
to learn what should be published, only to check it agrees.

## What `sync` takes away

When the configuration owns a host, `sync` also removes the one-host file
the old `expose` wrote for it, since Caddy refuses to reload with one
host named twice. A workspace whose last route was removed gets its
`<workspace>-routes.caddy` removed, not emptied — an empty file would
still serve.

Nothing else in `sites.d` is touched. A file another package added, or
written by hand, is not the configuration's to delete; `expose list`
reports it as `unmanaged` and says how to adopt it.

## Why DNS is still pointed by `add`

The DNS record is not in the configuration, and `sync` does not write it.
A name that does not resolve fails minutes later, in Caddy's certificate
log, where nobody is looking — so `add` points the name in the same
breath, printing the record to create by hand if the machine is
unreachable.
