# Publishing

Two ways to reach a port running on a machine: `expose` puts it on the
internet, `tunnel` reaches it privately. Neither is the default. The
question that picks between them is "who should reach it", not "which
protocol it speaks".

## The one thing to get right

| | Anyone | Only you |
| --- | --- | --- |
| **HTTP** | [`expose`](../reference/commands.md#expose) | [`tunnel`](../reference/commands.md#tunnel) |
| **Anything else** | nothing | [`tunnel`](../reference/commands.md#tunnel) |

Nothing in this CLI puts a non-HTTP port on the internet: `expose` only
writes an HTTPS hostname through Caddy. A database, a queue, or anything
else that isn't plain HTTP is always `tunnel`, whether or not it should be
reachable by anybody.

But "it speaks HTTP" doesn't mean "safe to publish". A database viewer
holding data restored from production, a captured-mail inbox showing mail
addressed to real customers, a queue dashboard that can drain a queue — all
three speak HTTP, none has a login of its own in front of it, and none
should go through `expose`. `tunnel` answers the same need — looking at
the thing from your computer — without ever putting a hostname in DNS.

## Where a published site is recorded

In `config.yml`, on the workspace, under `routes:`. `sync` writes it to the
machine; nothing reads the machine to learn what should be published. A
machine rebuilt from the configuration serves every site. The reasons are
in
[why a published site lives in the configuration](../how-it-works/published-sites.md).

## Why `expose` asks, and says who can reach it

`expose add` prints, in the question itself, that whatever is behind the
port becomes reachable by anybody who learns the hostname. That sentence
is the safeguard: reading "HTTP" as "safe" is exactly the habit that puts
a database viewer on the internet, and the last place to catch it is the
question asked right before it happens.

Automation must say `--publish` to cross that line. `--yes` only skips
ordinary write confirmations; it deliberately doesn't answer a publication
question, so a broad non-interactive flag can't turn a local change into a
public endpoint by accident. The same rule protects `dns add`.

## Why `expose` only speaks HTTPS

Caddy handles TLS for HTTP. Proxying raw TCP or UDP needs a plugin and a
custom build of Caddy — the same maintenance cost this project already
turned down for wildcard certificates. A port that isn't HTTP was never a
candidate for `expose`; it was always `tunnel`.
