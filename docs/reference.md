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

## Optional documentation tools

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
