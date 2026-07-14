# Elastic Docs Utils contributor guide

This repository builds `elastic-docs-utils`, a cross-harness installer for
Elastic documentation skills and MCP configuration.

## Layout

```text
cmd/elastic-docs-utils/  CLI entry point
internal/adapters/       Host-specific MCP and lifecycle integrations
internal/skills/         Shared skill catalog installation and safe links
internal/updates/        Cached, explicit update checks
internal/state/          Owned-artifact and preference persistence
```

## Development

```bash
GOCACHE=/tmp/docs-utils-go-build go test ./...
GOCACHE=/tmp/docs-utils-go-build go run ./cmd/elastic-docs-utils doctor
GOCACHE=/tmp/docs-utils-go-build go run ./cmd/elastic-docs-utils sync --dry-run
```

Do not overwrite an existing MCP configuration or skill directory unless this
project previously recorded ownership. Update checks must remain cache-only in
host lifecycle hooks; network refreshes are explicit user commands.
