# Elastic Docs Utils

Elastic Docs Utils configures a consistent documentation-authoring environment
for Claude Code, Codex, Cursor CLI, and OpenCode.

<img alt="Screenshot 2026-07-14 at 15 07 53" src="https://github.com/user-attachments/assets/86fb0243-042d-43a8-80e6-827d8373bcd9" />

It installs Elastic documentation skills into the shared `~/.agents/skills`
location, exposes them to hosts that need a discovery link, and configures the
public Elastic Docs MCP server. Elastic employees can opt into the internal
catalog and MCP server.

## Install

```bash
curl -sSL https://github.com/elastic/docs-utils/releases/latest/download/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://github.com/elastic/docs-utils/releases/latest/download/install.ps1 | iex
```

The installers download the matching release archive, verify its checksum, add
the binary to a standard user-accessible location, and run initial setup.

The installer detects available hosts. To choose explicitly:

```bash
elastic-docs-utils install --host claude,codex,cursor --internal
```

## Commands

```text
elastic-docs-utils install [--host <hosts>] [--internal] [--with-docs-tools|--with-vale|--with-docs-builder] [--yes] [--dry-run] [--force]
elastic-docs-utils sync [--host <hosts>] [--dry-run] [--force]
elastic-docs-utils status [--json|--quiet]
elastic-docs-utils check-updates [--json]
elastic-docs-utils update [--component all|skills,vale,vale-rules,docs-builder] [--force] [--dry-run]
elastic-docs-utils doctor [--json]
elastic-docs-utils uninstall [--purge] [--dry-run]
```

Automatic session notices only read the cached update result; they never make
network requests or modify tools. Run `check-updates` to refresh the cache and
`update` refreshes all updateable components by default. Use
`--component` with a comma-separated list to select `skills`, `vale`,
`vale-rules`, or `docs-builder`. Vale and Vale rules use the same maintained
upstream installer. Use `--force` to accept replacement prompts from those
installers.

Every non-dry-run `install` and `update` also refreshes the status of
docs-builder, the Vale binary, Elastic Vale rules, managed skills, and Elastic
Docs Utils.

For a complete first-time setup, use `--with-docs-tools`. It runs the
maintained installers for Vale, Elastic Vale rules, and docs-builder:

```bash
elastic-docs-utils install --with-docs-tools
```

Use `--with-vale` or `--with-docs-builder` when you want only one part of the
toolchain. These flags are explicit because the upstream installers can install
system tools and may ask before replacing existing local configuration.

To re-run the selected installers without their replacement prompts, pair the
option with `--force`:

```bash
elastic-docs-utils install --with-docs-tools --force
```

This confirms replacement of an existing non-Elastic Vale configuration and
overwrites an existing docs-builder binary. Vale itself remains managed by its
platform package manager when it is already installed; the Vale installer does
not forcibly replace that executable.

See the [command and managed-locations reference](docs/reference.md) for
`--verbose`, configuration paths, and optional-tool locations.

## What it manages

- Canonical skills at `~/.agents/skills`; Claude Code and Cursor receive links
  when required for discovery.
- Native MCP entries for Claude Code, Codex, Cursor CLI, and OpenCode.
- A small, private configuration and update cache under the OS config/cache
  directories in the `elastic/docs-utils` namespace.

Existing valid Elastic Docs MCP entries (including the established CloudFront
endpoint) and skill directories not created by Elastic Docs Utils are left
untouched. Use `--force` only when you want to replace a genuinely conflicting
Elastic Docs MCP entry with the canonical configuration. The installer also
removes only the retired `docs-harness` hook entries, leaving other Claude
settings alone. A binary started from a checkout or download is copied to
`/usr/local/bin` when writable, otherwise `~/.local/bin` (or the equivalent
per-user Windows location), before a Claude Code session hook is configured.
The hook uses that durable absolute path, so it does not depend on Claude Code
inheriting your shell PATH.

## Add a harness adapter

Support for another harness belongs in a focused adapter. See
[CONTRIBUTING.md](CONTRIBUTING.md#adding-a-harness-adapter) for the required
discovery, safe configuration, validation, tests, and documentation steps.

## Release

After merging a clean `main`, create a semantic-version release tag:

```bash
./scripts/release.sh 2.0.0
```

The tag triggers the release workflow. It builds macOS, Linux, and Windows
archives with GoReleaser and uploads `install.sh` and `install.ps1`, enabling
the curl and PowerShell installation commands above.

## License

Apache-2.0 — see [LICENSE.txt](LICENSE.txt).
