# docs-harness — contributor guide

This repo ships the **Elastic Docs Harness**: a one-time bootstrap installer
(Go binary) + a Claude Code plugin that together configure a complete Elastic
documentation authoring environment.

## Repo layout

```
cmd/docs-harness/        Go installer entrypoint
internal/
  config/                Harness config file + JSON merge helpers
  hooks/                 Embeds and extracts hook scripts to ~/.claude/docs-harness/
    scripts/             Embedded hook scripts (check-updates.sh, dispatch.sh)
  marketplace/           Registers Claude Code plugin marketplaces
  mcp/                   Ensures MCP server entries in ~/.claude.json
  preflight/             Prerequisite checks (Claude Code install, home dir)
  prompt/                Line-based interactive prompts + color helpers
  provider/              LiteLLM ↔ Claude toggle (settings.json env)
  telemetry/             OTel resource-attribute tagging
  tools/                 docs-builder + Vale version checks
.claude-plugin/          Plugin manifests (marketplace.json, plugin.json)
.mcp.json                Bundled MCP server definitions
commands/                Slash commands (docs-setup, docs-config, docs-update)
hooks/                   Hook scripts + rules (source of truth; embedded into binary)
templates/               CLAUDE.md.tmpl for docs repos
install.sh               POSIX installer shim
install.ps1              Windows installer shim
.goreleaser.yaml         Cross-compile + release config
.github/workflows/       CI: release.yml (tag → GitHub Release)
```

## Development

```bash
go mod tidy
go build ./...
go test ./...

# Run the installer locally (dry-run, no changes)
go run ./cmd/docs-harness --dry-run

# Build a snapshot release for local testing
goreleaser release --snapshot --clean
```

## Hook scripts

The hook scripts in `hooks/` are the **source of truth**. The Go installer
embeds the copies in `internal/hooks/scripts/` and extracts them to
`~/.claude/docs-harness/hooks/` at install time.

**When you edit a hook script**, also copy it to `internal/hooks/scripts/`:

```bash
cp hooks/check-updates.sh internal/hooks/scripts/
cp hooks/dispatch.sh internal/hooks/scripts/
cp hooks/dispatch-rules.json internal/hooks/scripts/
```

## Releasing

1. `git tag v1.x.y && git push origin v1.x.y`
2. GitHub Actions runs GoReleaser and publishes the release.
3. The `install.sh` / `install.ps1` shims automatically pull the new version.

## Architecture decisions

- **Installer is Go, hooks are bash**: Go gives safe JSON merging + true 3-OS
  cross-compilation; bash is sufficient for the in-Claude hook scripts which run
  in a shell Claude Code already provides.
- **Hooks at `~/.claude/docs-harness/hooks/`**: a stable, version-independent
  path. The installer overwrites these on every run so they stay in sync with
  the binary.
- **Orchestrate, don't bundle**: the harness registers and updates the
  `elastic-docs-skills` and `elastic-docs-skills-internal` marketplaces but
  does not copy their skills — they own their own distribution.
- **Telemetry via tagging only**: Claude Code's native OTel already streams to
  the org endpoint. We only add `OTEL_RESOURCE_ATTRIBUTES` + `OTEL_LOG_TOOL_DETAILS`
  so harness usage is filterable without re-owning the pipeline.
