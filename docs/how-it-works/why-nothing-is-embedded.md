# Why nothing is embedded in the binary

The CLI ships with no recipes inside it. Every package — including the ones
every machine gets — is fetched from a pinned release of
`github.com/adevmachine/packages`, verified against a checksum, and cached.

## What that buys

**Fixing how Docker is installed is a push, not a release.** Nobody upgrades a
binary to receive it, and the fix reaches every machine on the next `sync`.

**The CLI stays small enough to reason about.** It knows how to fetch a recipe,
send it to a machine and run it. It does not know what Docker is, and it never
needs to.

**One mechanism instead of two.** A recipe you wrote and a recipe somebody
published are the same kind of thing, in the same format, resolved by the same
code. There is no "built-in" set with different rules, so there is no second set
of rules to learn or to maintain.

## What it costs, plainly

**A machine cannot be built for the first time while GitHub is unreachable.**
There is nothing embedded to fall back on. You get:

```
error: fetching https://github.com/adevmachine/packages/releases/download/v1/packages-v1.tar.gz: ...
```

Once a release is in the cache this stops mattering: the recipes are on your
disk, `sync` copies them to the machine, and converging needs no network at all.
So the limit is narrower than it first looks — it is the **first** fetch of a
**new** pinned version, not every run.

This is a real limitation and it is worth the simplicity of one mechanism. If it
ever hurts, shipping a cache with the binary is the way out, and nothing about
the design has to change for that.

## Why the checksum is on a release asset

The CLI fetches `packages-v1.tar.gz` from the release, not the tarball GitHub
generates for the tag.

A tag can be moved. Its generated archive changes with it, silently, and
anything that trusted the tag is now running something else. A release asset is
uploaded once, and its checksum is published beside it, so `packages.lock` can
record what was actually installed and refuse content that changed.

Without that, "pinned" would be a word rather than a guarantee.

## Why only one source

A recipe runs as root on a machine you own. Fetching one from anywhere and
executing it is the shape of a supply-chain attack, so a third-party source is
not supported.

Your own configuration directory is the exception, and it is not really one: a
package you wrote is a package you already trust, and it never leaves your
machines.
