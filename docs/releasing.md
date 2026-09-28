# Releasing

A release is a tag. Everything else is automatic.

```
git tag vX.Y.Z
git push --tags
gh run watch          # follow the release workflow
```

Before tagging: `main` clean and pushed, CI green, `make test-vps` green
against the disposable Lima machine, and the local CLI acceptance gate green:

```
make accept
```

After those pre-tag gates, keep the release order: release the CLI first, wait
for packages CI, then release packages. Do not release packages ahead of the
CLI it is intended to accompany.

GoReleaser then builds the binaries, publishes them and updates the Homebrew
formula.

## The credential in place today

A GitHub App called `devmachine-release`, owned by the `mydevmachine`
organisation, installed on `homebrew-tap` only, with `Contents: Read and write`
and no other permission. Its App ID and private key are already stored as
secrets on `mydevmachine/devmachine`, and neither expires.

Regenerate the private key only if it leaks: the original download cannot be
repeated, but a new key can be generated and the old one revoked.

The rest of this page is how to rebuild that from scratch.

## One-time setup: the tap credential

The token GitHub injects into a workflow can only write to the repository the
workflow runs in. The Homebrew formula lives in another repository
(`mydevmachine/homebrew-tap`), so the release needs a credential that reaches it.

A **GitHub App** provides that. Unlike a personal access token it does not
expire, so a release cannot break a year from now because nobody rotated
anything.

### Create the App

1. In the organisation's settings → Developer settings → GitHub Apps → **New
   GitHub App**.
2. Name it something like `devmachine release`. Homepage URL can be the repo.
3. Uncheck **Webhook → Active**. It receives nothing.
4. Permissions → Repository permissions → **Contents: Read and write**. Nothing
   else.
5. Create it, then note the **App ID**.
6. **Generate a private key** and download the `.pem`.
7. **Install App** → the organisation → **Only select repositories** →
   `homebrew-tap`.

### Store the credentials

```
gh secret set TAP_APP_ID --repo mydevmachine/devmachine
gh secret set TAP_APP_PRIVATE_KEY --repo mydevmachine/devmachine < path/to/key.pem
```

Organisation secrets work too, and are worth it if more repositories will
publish to the tap:

```
gh secret set TAP_APP_ID --org mydevmachine --visibility all
```

### Check it

```
gh secret list --repo mydevmachine/devmachine
```

Both names should be listed. Without them the release workflow fails at the
token step, before building anything.

## What a release produces

- `tar.gz` archives for darwin and linux, amd64 and arm64
- `checksums.txt`
- release notes from the commit subjects, with `docs:`, `test:` and `chore:`
  left out
- an updated formula in the tap, installing `devmachine` and `advm`

## The documentation site

https://mydevmachine.sh/ is built by the `mydevmachine/docs`
repository from this repository's `docs/` on `main`. It rebuilds every six
hours on its own. After pushing a change under `docs/`, or after a release,
rebuild it now:

```
gh workflow run deploy.yml -R mydevmachine/docs
gh run watch -R mydevmachine/docs $(gh run list -R mydevmachine/docs --limit 1 --json databaseId -q '.[0].databaseId')
```

The skills in `mydevmachine/packages` carry a copy of the reference pages. After
a release that changed them, regenerate that copy there with
`scripts/sync-skill-references.sh <path-to-this-release's-docs>`; the packages
CI fails until it matches the latest release.

## Verify

```
brew install mydevmachine/tap/devmachine
devmachine version
```
