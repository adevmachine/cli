# What sync removes

`sync` only removes a file it recorded writing. Never a guess, never a
whole directory swept clean — one path, remembered, removed once the plan
stops writing it.

## Two kinds of file, one rule

A workspace's routes and a package's `extends` both write into `sites.d`,
and `sync` removes both the same way: it keeps a list of exactly what it
wrote, and takes away whatever is no longer on that list.

- **Routes** come from `expose`. See
  [Why a published site lives in the configuration](published-sites.md)
  for how `sync` tracks and removes them.
- **Extension files** come from `extends: caddy.sites.d: <file>`, a
  package's way of adding to a place another package opened. `sync`
  records the absolute path of every file an extension wrote, in the
  lock, per machine. The next time a package that used to extend
  `caddy.sites.d` is no longer in the plan, `sync` removes the file it
  left behind and reloads Caddy.

## Why not a glob

`sites.d` can hold files nothing in the configuration wrote: one dropped
in by hand, one from a package that predates this feature. A glob over the
directory would delete those too. `sync` only removes a path it remembers
writing itself, so a file it never touched is never at risk.

## The first-run caveat

The list of extension paths a machine's last sync wrote lives in the lock.
A lock from before this feature has no such list, so the first sync after
upgrading removes nothing — there is nothing yet to compare against.

That also means a file left by a package removed *before* this version
never enters the tracked list, so no later sync ever catches it either:
nothing was ever recorded writing it. Remove it by hand with `devmachine
run`. See [Troubleshooting](../troubleshooting.md) for the exact steps.
