# Getting started

You need a Debian or Ubuntu VPS you can reach as root over SSH, and a Mac or
Linux computer. Three commands:

```
brew install adevmachine/tap/devmachine
devmachine setup
devmachine workspaces new alice && devmachine sync
```

Then work in it:

```
devmachine ssh alice
```

`setup` asks for the machine's address and shows its host key fingerprint
before trusting it — compare it with your provider's console. It installs a
key, checks the key works, and turns password login off. `workspaces new`
adds a Linux account to your configuration, and `sync` builds it on the
machine. On Linux without Homebrew, take the binary from the
[releases page](https://github.com/adevmachine/cli/releases).

Something failed? [Troubleshooting](troubleshooting.md) says what each error
really means.

## When you want more

- **Sign in to GitHub once, for every workspace:** `devmachine login gh`, then
  `devmachine credentials push`. See [credentials](concepts/credentials.md).
- **Put a port on the internet:** `devmachine expose add alice 3000 --host app.example.com`,
  then `devmachine sync`. See [publishing](concepts/publishing.md).
- **Teach your coding agent the CLI:** `devmachine skills add`. See
  [agent skills](concepts/agent-skills.md).
- **Keep your configuration in git:** `devmachine setup git`. See
  [versioning your configuration](how-it-works/versioning-your-configuration.md).
- **Add tools to a workspace:** `devmachine packages add <name> --workspace alice`,
  then `devmachine sync`. See [packages](concepts/packages.md).
