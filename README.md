# Elastic Docs Harness

Turnkey Claude Code environment for Elastic documentation contributors.

One command configures:
- **MCP servers** — Elastic Docs (public) and Elastic Internal Docs
- **Skill catalogs** — `elastic-docs-skills` and `elastic-docs-skills-internal`
- **Provider toggle** — switch between in-house Claude and the Elastic LiteLLM gateway
- **Soft skill dispatch** — context-aware skill suggestions on docs edits and prompts
- **Update checks** — quiet session-start notices when docs-builder, Vale, or skills are behind

## Install

```bash
curl -sSL https://github.com/elastic/docs-harness/releases/latest/download/install.sh | bash
```

Windows (PowerShell):
```powershell
irm https://github.com/elastic/docs-harness/releases/latest/download/install.ps1 | iex
```

Restart Claude Code after installation.

## What it does

The installer:
1. Checks that Claude Code CLI is installed (offers to install it via `npm` if not)
2. Registers the `elastic-docs-skills`, `elastic-docs-skills-internal`, and `docs-harness` Claude Code marketplaces
3. Installs all three plugins user-scoped
4. Adds the two Elastic Docs MCP servers to `~/.claude.json`
5. Configures your preferred API provider (in-house Claude or LiteLLM gateway)
6. Adds OTel resource-attribute tagging (`team=docs`) to the existing org pipeline
7. Extracts the hook scripts and registers them in `~/.claude/settings.json`
8. Checks `docs-builder` and `vale` versions against latest releases

## Commands (inside Claude Code)

| Command | Description |
|---|---|
| `/docs-setup` | Full interactive (re)configuration |
| `/docs-config [provider\|dispatch\|catalogs\|telemetry]` | Quick toggles |
| `/docs-update` | Check and apply tool + skill updates |

## Options

```
docs-harness [options]

  --provider claude|litellm   API provider (default: prompted)
  --gateway-url URL           LiteLLM gateway base URL
  --catalogs public|both      Skill catalogs (default: both)
  --no-telemetry              Disable OTel resource-attribute tagging
  --yes                       Non-interactive, accept all defaults
  --dry-run                   Show what would be done without making changes
  --force                     Overwrite existing MCP entries
  --version                   Print version
```

## License

Apache 2.0 — see [LICENSE.txt](LICENSE.txt).
