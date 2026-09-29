# Publishing

Two ways to reach a port running on a machine: `expose` puts it on the
internet, `tunnel` reaches it only from your computer. Neither is the
default — you pick based on who should reach it.

```
devmachine expose add acme 3000 --host app.example.com
devmachine tunnel acme 5432
```

## Which one to use

| | Anyone | Only you |
| --- | --- | --- |
| **HTTP** | [`expose`](../reference/commands.md#expose) | [`tunnel`](../reference/commands.md#tunnel) |
| **Anything else** | nothing | [`tunnel`](../reference/commands.md#tunnel) |

`expose` only works for HTTP, and only puts up an HTTPS address — nothing
else goes on the internet. `tunnel` works for anything, HTTP or not, and
never touches DNS.

Speaking HTTP doesn't mean "safe to publish". A database viewer, a
captured-mail inbox, a queue dashboard that can drain a queue: all three
speak HTTP, none has its own login, and none should go through `expose`.
Use `tunnel` for those — it looks at the thing from your computer,
without ever making it public.

## Where a published site is recorded

In `config.yml`, on the workspace, under `routes:`. `sync` is what
actually writes it to the machine. See
[why a published site lives in the configuration](../how-it-works/published-sites.md).

## Why `expose` asks first

`expose add` tells you, before it does anything, that the port becomes
reachable by anybody who learns the hostname. In a script, pass
`--publish` to confirm that on purpose — `--yes` alone never does. The
same rule protects `dns add`.
