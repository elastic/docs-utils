---
name: docs-update
description: Check for and apply updates to docs-builder, Vale rules, and the Elastic Docs skill catalogs. Shows what's behind and offers to update each one.
argument-hint: (no arguments needed)
context: fork
allowed-tools: Read, Bash(docs-harness *), Bash(claude plugin *), Bash(curl *), Bash(which *), Bash(vale *), Bash(docs-builder *), Bash(powershell *)
---

<!-- Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
or more contributor license agreements. Licensed under the Apache License, Version 2.0. -->

# /docs-update — Check and apply updates

Use this skill to check whether docs-builder, the Elastic Vale rules, and your skill catalogs are up to date, and to apply any available updates.

## Steps

1. **Check docs-builder version:**
   ```bash
   docs-builder --version 2>&1 || docs-builder 2>&1 | head -1
   ```
   Fetch the latest release tag from `https://api.github.com/repos/elastic/docs-builder/releases/latest` and compare `tag_name` (strip leading `v`).

2. **Check Elastic Vale rules version:**
   The installed version is **not** the `vale` binary version — it is stored in the `VERSION` file inside the Elastic styles directory.
   ```bash
   # Try these paths in order until one exists:
   cat "$VALE_STYLES_PATH/Elastic/VERSION" 2>/dev/null \
     || cat "$(vale ls-dirs 2>/dev/null | awk '/StylesPath/{print $3}')/Elastic/VERSION" 2>/dev/null \
     || cat "$HOME/Library/Application Support/vale/styles/Elastic/VERSION" 2>/dev/null \
     || cat "$HOME/.local/share/vale/styles/Elastic/VERSION" 2>/dev/null \
     || echo "(not installed)"
   ```
   Fetch the latest release tag from `https://api.github.com/repos/elastic/vale-rules/releases/latest` and compare `tag_name`.

3. **Check elastic-docs-skills version:**
   Read the installed version from `~/.claude/plugins/installed_plugins.json` (key `elastic-docs-skills@elastic-docs-skills`, field `version`). Fetch the latest version from `https://raw.githubusercontent.com/elastic/elastic-docs-skills/main/.claude-plugin/plugin.json` (field `version`). Compare the two.

4. **Report findings.** Show a clear table of installed vs latest for each item. Example:
   ```
   docs-builder          1.16.1   →   1.17.7  ⚠ update available
   elastic-vale-rules    1.4.0    →   1.5.0   ⚠ update available
   elastic-docs-skills            ✓ up to date
   ```

5. **For each item that is behind, offer to update:**

   **docs-builder** — re-run the official installer (handles macOS and Linux):
   ```bash
   curl -sL https://ela.st/docs-builder-install | sh
   ```
   Windows (PowerShell):
   ```powershell
   iex (New-Object System.Net.WebClient).DownloadString('https://ela.st/docs-builder-install-win')
   ```

   **Elastic Vale rules** — re-run the official installer for the user's OS:

   macOS:
   ```bash
   curl -fsSL https://raw.githubusercontent.com/elastic/vale-rules/main/install-macos.sh | bash
   ```
   Linux:
   ```bash
   curl -fsSL https://raw.githubusercontent.com/elastic/vale-rules/main/install-linux.sh | bash
   ```
   Windows (PowerShell):
   ```powershell
   Invoke-WebRequest -Uri https://raw.githubusercontent.com/elastic/vale-rules/main/install-windows.ps1 -OutFile install-windows.ps1
   powershell -ExecutionPolicy Bypass -File .\install-windows.ps1
   ```

   **elastic-docs-skills:**
   ```bash
   claude plugin marketplace update
   claude plugin update elastic-docs-skills@elastic-docs-skills --scope user
   ```

6. **Apply chosen updates** (run the command above for each item the user confirms) and report what was updated.

## Notes

- The Elastic Vale rules version (`elastic/vale-rules`) is separate from the `vale` binary version (`errata-ai/vale`). Always check and report the rules version, not the binary version.
- The install scripts for both tools are idempotent — re-running them is safe and is the correct update path.
- This is the interactive version of the `docs-harness check-updates` command used by the SessionStart hook. The hook only prints a notice when something is behind; this command shows the full picture and applies updates.
