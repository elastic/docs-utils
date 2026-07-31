# Elastic Docs Utils reference

## Global options

| Option | Meaning |
|---|---|
| `--color auto\|always\|never` | Control ANSI color output. |
| `--verbose` | Print every Elastic Docs Utils-managed file or directory changed by a mutating command. It prints paths only, never configuration values or secrets. |

Global options can appear before or after the command:

```bash
elastic-docs-utils --verbose install --with-vale
elastic-docs-utils install --verbose --with-docs-builder
elastic-docs-utils install --verbose --with-docs-tools
```

## Managed locations

| Purpose | Location |
|---|---|
| Preferences | macOS/Linux: OS config directory under `elastic/docs-utils/config.json`; Windows: `%AppData%\elastic\docs-utils\config.json` |
| Managed state | macOS/Linux: OS config directory under `elastic/docs-utils/state.json`; Windows: `%AppData%\elastic\docs-utils\state.json` |
| Update cache | OS cache directory under `elastic/docs-utils/update-status.json` |
| Shared skills | `~/.agents/skills/<skill>` |
| Claude Code MCP | `~/.claude.json` |
| Claude Code session hook | `~/.claude/settings.json` |
| Codex MCP | `~/.codex/config.toml` |
| Cursor CLI MCP | `~/.cursor/mcp.json` |
| OpenCode MCP | macOS/Linux: `~/.config/opencode/opencode.json`; Windows: the OS config directory under `opencode/opencode.json` |

Claude Code and Cursor CLI receive skill discovery links below their respective
`~/.claude/skills` and `~/.cursor/skills` directories. Existing non-Elastic
configuration is retained unless `--force` is explicitly supplied for a
conflicting Elastic Docs MCP entry.

Before configuring host adapters, `install`, `sync`, and `update` place a
binary started from a checkout or download into `/usr/local/bin` when writable,
or `~/.local/bin` otherwise. On Windows, the location is
`%LOCALAPPDATA%\Elastic\DocsUtils`. The Claude Code session hook records this
durable absolute path, so it works even when Claude Code starts without the
shell PATH.

## Optional documentation tools

`install --with-docs-tools` is the recommended first-time setup option. It runs
both upstream installers, ensuring Vale is present, installing the Elastic Vale
rules, and installing docs-builder. The narrower `--with-vale` and
`--with-docs-builder` options run only the corresponding installer.

Pair any of these options with `--force` to accept the upstream installers'
replacement prompts. This replaces a non-Elastic Vale configuration and
overwrites docs-builder when it already exists. The upstream Vale installer
keeps an existing Vale executable package-managed; it refreshes the Elastic
rules rather than forcibly replacing the executable.

`install --with-vale` runs the maintained Elastic Vale Rules platform
installer. It may install the Vale binary and manages the following locations:

| Platform | Vale configuration | Elastic styles |
|---|---|---|
| macOS | `~/Library/Application Support/vale/.vale.ini` | `~/Library/Application Support/vale/styles/Elastic` |
| Linux | `~/.config/vale/.vale.ini` | `~/.local/share/vale/styles/Elastic` (or XDG overrides) |
| Windows | `%LOCALAPPDATA%\vale\.vale.ini` | `%LOCALAPPDATA%\vale\styles\Elastic` |

`install --with-docs-builder` runs the maintained Docs Builder installer. Its
installation location is chosen by that upstream script and is printed by the
script while it runs.

## Updates

`update` refreshes all updateable components by default:

```bash
elastic-docs-utils update
```

Select specific components with a comma-separated `--component` value:

```bash
elastic-docs-utils update --component skills,vale-rules,docs-builder
```

Supported components are `skills`, `vale`, `vale-rules`, and `docs-builder`.
The `vale` and `vale-rules` selections use the same upstream installer because
it manages both components together. Add `--force` to accept replacement
prompts from upstream installers.
