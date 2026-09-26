# Why a published site lives in the configuration

`expose add` used to write a Caddy file straight onto the machine and reload
Caddy. It worked, and it left the only record of the site on the machine.
Rebuild the machine from the configuration and every package came back, the
routes in a package's own files came back, and the sites `expose` had written
did not — because nothing but the machine ever knew about them.

So the record moved. A route is a field of the workspace that owns it:

    workspaces:
      - name: alice
        routes:
          - {host: app.example.com, port: 8080}

`expose add` writes that line. `sync` renders one file per workspace,
`alice-routes.caddy`, into the `sites.d` that the `caddy` package provides,
and reloads Caddy when the file changed. `expose rm` deletes the line and the
next `sync` deletes the block. There is one direction: configuration to
machine. The machine is never read to learn what should be published, only to
check that it agrees.

## What `sync` takes away

When the configuration owns a host, `sync` also removes the one-host file the
old `expose` wrote for it, `<host>.caddy`. Caddy refuses a reload that names
one host twice, so adopting a site could not leave both files in place.

A workspace whose last route was removed gets its `<workspace>-routes.caddy`
removed, not emptied: an empty file would still exist, and a stale one would
still serve.

Nothing else in `sites.d` is touched. A file another package contributed, or
one written by hand, is not the configuration's to delete; `expose list`
reports it as `unmanaged` and says how to adopt it.

## Why DNS is still pointed by `add`

The DNS record is not in the configuration and `sync` does not write it. A
name that does not resolve fails minutes later, in Caddy's certificate log,
where nobody is looking — so `add` points the name in the same breath, and
with the machine unreachable prints the record to create by hand rather than
losing it.
