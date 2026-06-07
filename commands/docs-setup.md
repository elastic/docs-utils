---
name: docs-setup
description: Set up or reconfigure the Elastic Docs Harness — register MCP servers, install skill catalogs, configure the provider, and check tool versions. Run this the first time or to change your configuration.
argument-hint: (no arguments needed)
context: fork
allowed-tools: Read, Edit, Write, Bash(docs-harness *), Bash(claude plugin *)
---

<!-- Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
or more contributor license agreements. Licensed under the Apache License, Version 2.0. -->

# /docs-setup — Elastic Docs Harness setup

This skill guides you through configuring or reconfiguring the Elastic Docs Harness interactively. It reads the current harness config, asks what you want to change, and applies the changes.

## Steps

1. **Read current config.** Read `~/.claude/docs-harness.json` if it exists to show current settings.

2. **Ask what to configure.** Present the user with the current state and ask which of these they want to set up or change:
   - Provider (in-house Claude vs LiteLLM gateway)
   - Skill catalogs (public only vs public + internal)
   - Soft skill dispatch (on/off)
   - Telemetry tagging (on/off)
   - Drop a `CLAUDE.md` template into the current repo

3. **Apply changes** by running the installer binary:
   ```bash
   docs-harness --yes [flags based on user choices]
   ```
   If `docs-harness` is not on PATH, instruct the user to run the installer first:
   ```
   curl -sSL https://github.com/elastic/docs-harness/releases/latest/download/install.sh | bash
   ```

4. **Confirm.** Show the updated `~/.claude/docs-harness.json` and remind the user that provider changes take effect on the next Claude Code session restart.

5. **CLAUDE.md template.** If the user wants to drop a docs repo template, copy `~/.claude/plugins/cache/docs-harness/docs-harness/*/templates/CLAUDE.md.tmpl` to `./CLAUDE.md` in the current working directory (only if one doesn't already exist, or after confirming overwrite).

## Notes

- The harness config is `~/.claude/docs-harness.json` — this is the runtime source of truth for the hooks.
- The `settings.json` env block holds `ANTHROPIC_BASE_URL` (for LiteLLM) and OTel tags.
- The `~/.claude.json` file holds the MCP server entries.
- Changes to provider or MCP servers require a Claude Code session restart to take effect.
