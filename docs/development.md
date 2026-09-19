# Development

```
make build        compile to ./devmachine
make test         the whole suite
make test-vps     the suite against a real machine
make lint         golangci-lint
make fmt          gofmt
make surface      regenerate SURFACE.txt
```

## Testing against a real machine

Tests that touch a machine run against a throwaway VPS on this computer, never
against a real server. It needs Lima (`brew install lima`).

```
make test-vps
```

That starts the VM, authorizes a throwaway key and runs everything against it.
The VM reproduces a freshly bought server: root over SSH with a password and no
key installed.

The VM is made by the CLI itself — `devmachine machines create-local`, see
[the reference](reference/commands.md#machines) — so the harness and the
feature are one thing and cannot drift apart.

One difference is deliberate: the script then authorizes a key through
`limactl shell`, and `create-local` never does. The integration tests need a
machine they can already reach without running `setup` first, while
`create-local` has to leave the machine exactly as a bought one arrives, or the
trust bootstrap is never exercised by the thing that runs on every test.

Driving it by hand:

```
scripts/fake-vps.sh up      start it and authorize a key
scripts/fake-vps.sh env     the values a test needs, as exports
scripts/fake-vps.sh ssh     a shell on it
scripts/fake-vps.sh down    destroy it
```

Without the VM those tests **skip**, so `make test` stays green anywhere —
including CI, which has no VM.

`internal/local` goes further: where Lima is installed, its test creates a
machine and proves that root gets in with the password and no key does. That
costs a boot, so `go test -short` leaves it out.

## Rules this repository holds itself to

**Everything written to disk is English.** Code, comments, commit messages,
documentation, fixtures.

**No real infrastructure or personal data, ever.** No real machine names,
hostnames, IPs, domains, emails or tokens — not in code, tests, fixtures, docs
or commit messages. Fixtures use `alice` and `bob`, `example.com`, and the
documentation IP ranges `203.0.113.x` and `198.51.100.7`.

`scripts/check-no-real-data.sh` enforces this against a pattern list kept
outside the repository, and runs in CI. It has already caught a real leak.

**A test must not depend on the machine running it.** A test that passes on a
laptop with an SSH agent and fails on a runner without one is testing the
environment. Generate what the test needs.

**Documentation is part of the change.** See below.

## Documentation

`docs/` is the manual, and it is updated in the same commit as the behaviour it
describes. Specifically:

- A new command or flag goes in [the reference](reference/commands.md).
- A decision that is **not obvious from the outside** goes in `how-it-works/`.
  If explaining something took a paragraph in a pull request or a conversation,
  that paragraph belongs here.
- An error somebody could meet goes in [troubleshooting](troubleshooting.md),
  with what it really means — not just what it says.
- Every new page is linked from [the index](index.md). A page nothing links to
  does not exist.

The reason for the second rule: the hard-won parts of this project are not the
code, they are the reasons. Why one key is offered and never the whole agent,
why a dropped address is not an error, why a command refuses to guess. That
knowledge is worth more written down than rediscovered.

## Specs and plans

Design documents and implementation plans are **not** in this repository. They
live in the operator's own repo and are gitignored here.

`docs/` is for people using the CLI. That is a different audience and a
different document.
