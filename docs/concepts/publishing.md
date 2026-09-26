# Publishing

Two ways to reach a port that is running on a machine: `expose` puts it on
the internet, `tunnel` reaches it privately. Neither is the default; the
question that picks between them is "who should reach it", not "which
protocol it speaks".

## The one thing to get right

An admin panel speaks HTTP and still must not be published. A database
studio holding data restored from production, a captured-mail inbox showing
mail addressed to real customers, a queue dashboard that can drain a queue —
all three speak HTTP, none has authentication of its own in front of it, and
"HTTP goes through `expose`" is how one of them reaches the internet.

| | Anyone | Only you |
| --- | --- | --- |
| **HTTP** | [`expose`](../reference/commands.md#expose) | [`tunnel`](../reference/commands.md#tunnel) |
| **Anything else** | nothing | [`tunnel`](../reference/commands.md#tunnel) |

Nothing in this CLI puts a non-HTTP port on the internet: `expose` only
writes an HTTPS hostname through Caddy. A database, a queue, or anything
else that is not plain HTTP is always `tunnel`, whether or not it should be
reachable by anybody.

## Three real cases, and why they are `tunnel`

- **A database studio** holding data restored from production. Anybody who
  finds the hostname can read every row.
- **A captured-mail inbox**, the kind a development environment uses instead
  of sending real mail, showing mail addressed to real customers.
- **A queue dashboard** that can drain a queue — reading is one click away
  from acting.

All three speak HTTP. All three could, technically, go through `expose`.
None of them should: none has its own login, so whoever reaches the
hostname reaches the data. `tunnel` answers the same need — looking at the
thing from this computer — without ever putting a hostname in DNS.

## Where a published site is recorded

In `config.yml`, on the workspace, under `routes:`. `sync` writes it to the
machine; nothing reads the machine to learn what should be published. A
machine rebuilt from the configuration serves every site. The reasons are in
[why a published site lives in the configuration](../how-it-works/published-sites.md).

## Why `expose` asks, and says who can reach it

`expose add` prints, in the question itself, that whatever is behind the
port becomes reachable by anybody who learns the hostname. That sentence is
the safeguard: reading "HTTP" as "safe to publish" is exactly the habit that
puts a database studio on the internet, and the last place to catch it is
the question asked right before it happens.

Automation must say `--publish` to cross that boundary. `--yes` only skips
ordinary write confirmations and deliberately does not answer a publication
question; this keeps a broad non-interactive flag from turning a local change
into a public endpoint by accident. The same rule protects `dns add`.

## Why `expose` only speaks HTTPS

Caddy terminates TLS for HTTP. Proxying raw TCP or UDP needs a plugin and a
custom build of Caddy — the same maintenance cost this project already
refused for wildcard certificates and DNS-01. A port that is not HTTP was
never a candidate for `expose`; it was always `tunnel`.
