---
name: docs-config
description: Quickly toggle Elastic Docs Harness settings — switch between LiteLLM and in-house Claude, enable/disable skill dispatch, change catalogs, or toggle telemetry tagging.
argument-hint: "[provider|dispatch|catalogs|telemetry]"
context: fork
allowed-tools: Read, Edit, Write, Bash(docs-harness *), Bash(claude plugin *)
---

<!-- Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
or more contributor license agreements. Licensed under the Apache License, Version 2.0. -->

# /docs-config — Quick harness configuration

Use this skill for quick configuration changes without going through the full setup flow.

## Usage patterns

`/docs-config` — show current config and offer to change any setting
`/docs-config provider` — toggle between in-house Claude and LiteLLM
`/docs-config dispatch` — enable/disable or change soft dispatch mode
`/docs-config catalogs` — change which skill catalogs are active
`/docs-config telemetry` — toggle OTel resource-attribute tagging

## Steps

1. **Read current config** from `~/.claude/docs-harness.json`.

2. **Based on the argument** (or by asking if none provided), make the requested change:

   **provider**: Ask "in-house Claude or LiteLLM gateway?". For LiteLLM ask for the gateway URL. Run:
   ```bash
   docs-harness --provider [claude|litellm] --gateway-url [URL] --yes
   ```

   **dispatch**: Ask "enable or disable soft skill suggestions?". Update `docs-harness.json` dispatch.enabled field:
   ```bash
   # Edit ~/.claude/docs-harness.json directly — just the dispatch block
   ```

   **catalogs**: Ask "public only, or public + internal?". Run:
   ```bash
   docs-harness --catalogs [public|both] --yes
   ```

   **telemetry**: Ask "enable or disable OTel tagging?". Run:
   ```bash
   docs-harness --telemetry --yes        # enable
   docs-harness --no-telemetry --yes     # disable
   ```

3. **Confirm** the change and note whether a session restart is needed:
   - Provider changes: **restart required**
   - Dispatch changes: take effect immediately (hooks read the config on each invocation)
   - Telemetry changes: take effect on next session start

## Current settings display

When run with no argument, show a table like:
```
Provider:   in-house Claude
Catalogs:   public, internal
Dispatch:   enabled (soft)
Telemetry:  enabled
Version:    1.0.0
```
