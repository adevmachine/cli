# Why a published site lives in the configuration

`expose add` used to write a Caddy file straight onto the machine and
reload Caddy. It worked, but it left the only record of the site on the
machine: rebuild from the configuration, and the sites `expose` had
written never came back.

So the record moved. A route is a field of the workspace that owns it:

    workspaces:
      - name: acme
        routes:
          - {host: app.example.com, port: 8080}

`expose add` writes that line. `sync` renders one file per workspace,
`acme-routes.caddy`, into the `sites.d` folder the `caddy` package
provides, and reloads Caddy when the file changed. `expose rm` deletes
the line. There is one direction — configuration to machine — devmachine
never reads the machine to learn what should be published, only to check
it agrees.

## Why `expose` does not wait for `sync`

`sync` runs the machine's whole play: every package, every workspace. That
takes minutes, and a new subdomain should take seconds. So `expose add`
and `expose rm` also apply the one file the route changes, right after
they record it:

1. Render the workspace's routes file from `config.yml` with the same
   code `sync` uses. Same bytes, same path, owner `root:root`, mode
   `0644` — so the next `sync` finds nothing to change.
2. Write it next to the real file, under a name the Caddyfile's
   `sites.d/*.caddy` import does not match.
3. Set the old file aside (and any one-host file the old `expose` wrote
   for one of these hosts), and move the new one in.
4. Run `caddy validate` on the whole `/etc/caddy/Caddyfile`. The new file
   has to be in place for this: nothing else would check the full
   configuration with it.
5. Reload Caddy (`systemctl reload caddy`, then `caddy reload` if that
   fails), and delete what was set aside.

If Caddy refuses the file in step 4 or 5, the old files go back, Caddy
never loads the new one, and the command prints Caddy's own error. Caddy
reads the files only on a reload, so the short time the new file sits in
place changes nothing it serves.

Only that workspace's file is touched, and it is rendered from the whole
configuration, not patched: if the machine's copy has drifted, it comes
back to what `config.yml` says. Other workspaces' files wait for `sync`.

It runs over the same SSH connection `devmachine run` shares, as the
machine's admin login (through `sudo -n` when that login is not root). A
machine that cannot be reached, or a machine with `caddy` in the
configuration but not installed yet, is not an error: the route stays
`pending` and `sync` publishes it. A [self machine](your-computer-as-a-machine.md)
always waits for `sync`.

`--no-apply` skips all of this and only records the route.

## What `sync` takes away

When the configuration owns a host, `sync` also removes the one-host file
the old `expose` wrote for it, since Caddy refuses to reload with one
host named twice. A workspace whose last route was removed gets its
`<workspace>-routes.caddy` removed, not emptied — an empty file would
still serve.

Nothing else in `sites.d` is touched. A file another package added, or
written by hand, is not the configuration's to delete; `expose list`
reports it as `unmanaged` and says how to adopt it.

A file another package adds to `sites.d` — like the one `caddy-sites`
writes — also reloads Caddy when `sync` changes or removes it. Without that
reload, Caddy would keep serving the old file until something else
reloaded it.

## Why DNS is still pointed by `add`

The DNS record is not in the configuration, and `sync` does not write it.
A name that does not resolve fails minutes later, in Caddy's certificate
log, where nobody is looking — so `add` points the name in the same
breath, printing the record to create by hand if the machine is
unreachable.
