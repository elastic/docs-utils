#!/usr/bin/env bash
# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Apache License, Version 2.0.
#
# Soft skill-dispatch hook.
# Called on UserPromptSubmit and PostToolUse (Edit|Write|NotebookEdit).
# Reads dispatch-rules.json and docs-harness.json; injects a one-line skill
# suggestion into context when a rule matches. Never blocks.
#
# Context is received via stdin as JSON from Claude Code hooks.

set -euo pipefail

HARNESS_CONFIG="${HOME}/.claude/docs-harness.json"
RULES_FILE="${HOME}/.claude/docs-harness/hooks/dispatch-rules.json"
HOOK_EVENT="${HOOK_EVENT:-}"

# Exit if dispatch is disabled.
if command -v python3 &>/dev/null && [[ -f "$HARNESS_CONFIG" ]]; then
  enabled=$(python3 -c "
import json, sys
try:
    d = json.load(open('$HARNESS_CONFIG'))
    dispatch = d.get('dispatch', {})
    print('true' if dispatch.get('enabled', True) else 'false')
except Exception:
    print('true')
")
  [[ "$enabled" == "false" ]] && exit 0
fi

# Read hook context from stdin.
context=$(cat 2>/dev/null || true)

if [[ -z "$context" ]]; then
  exit 0
fi

dispatch_on_prompt() {
  local ctx="$1"
  if ! command -v python3 &>/dev/null; then return; fi

  python3 - "$ctx" <<'PYEOF'
import json, sys, re

ctx_str = sys.argv[1]
try:
    ctx = json.loads(ctx_str)
except Exception:
    sys.exit(0)

prompt_text = ctx.get("prompt", "").lower()

rules = [
    # (regex pattern, skill name, suggestion text)
    (r"\bchangelog\b", "fix-changelog",   "consider using /fix-changelog to improve the changelog entry"),
    (r"\bchangelog\b", "review-changelog","or /review-changelog to audit it"),
    (r"\bapplies.?to\b|applies_to\b",     "applies-to-tagging",  "consider /applies-to-tagging to validate applies_to metadata"),
    (r"\bcontent.?type\b|content type\b", "content-type-checker","consider /content-type-checker to validate content type compliance"),
    (r"\bfrontmatter\b|front.?matter\b",  "frontmatter-audit",   "consider /frontmatter-audit to check frontmatter completeness"),
    (r"\bjargon\b|internal.?term",        "flag-jargon-skill",   "consider /flag-jargon-skill to catch internal jargon"),
    (r"\bllm.?matrix\b|llm performance",  "llm-matrix-update",   "consider /llm-matrix-update for LLM benchmark changes"),
]

suggestions = []
for pattern, skill, text in rules:
    if re.search(pattern, prompt_text):
        suggestions.append(f"💡 Docs skill hint: {text} (`/{skill}`).")
        break  # one suggestion per prompt

if suggestions:
    # Output as a Claude Code hook response (inject into context)
    print("\n".join(suggestions))
PYEOF
}

dispatch_on_file_edit() {
  local ctx="$1"
  if ! command -v python3 &>/dev/null; then return; fi

  python3 - "$ctx" <<'PYEOF'
import json, sys, re, fnmatch

ctx_str = sys.argv[1]
try:
    ctx = json.loads(ctx_str)
except Exception:
    sys.exit(0)

# The edited file path is in tool_input.file_path for Edit/Write tools.
tool_input = ctx.get("tool_input", {})
file_path = tool_input.get("file_path", "") or tool_input.get("path", "")

if not file_path:
    sys.exit(0)

rules = [
    # (glob pattern, skill name, suggestion text)
    ("*/docs-content*/*.md",   "docs-check-style",       "consider /docs-check-style to run Vale style checks on this file"),
    ("*/docs-content*/*.md",   "applies-to-tagging",     "and /applies-to-tagging to validate applies_to metadata"),
    ("*/docs-content*/*.md",   "crosslink-validator",    "and /crosslink-validator to check cross-links"),
    ("*changelog*.yaml",       "review-changelog",       "consider /review-changelog to audit this changelog entry"),
    ("*changelog*.yml",        "review-changelog",       "consider /review-changelog to audit this changelog entry"),
]

suggestions = set()
for pattern, skill, text in rules:
    if fnmatch.fnmatch(file_path, pattern) and skill not in suggestions:
        # Only emit one suggestion per tool call to avoid noise.
        if "docs-content" in file_path and file_path.endswith(".md"):
            print(f"💡 Docs skill hint: {text} (`/{skill}`).")
            suggestions.add(skill)
            break  # one suggestion per edit
        elif ("changelog" in file_path) and (file_path.endswith(".yaml") or file_path.endswith(".yml")):
            print(f"💡 Docs skill hint: {text} (`/{skill}`).")
            suggestions.add(skill)
            break
PYEOF
}

# ── Main: dispatch based on the hook event type ──────────────────────────────
if [[ "$HOOK_EVENT" == "UserPromptSubmit" ]]; then
  dispatch_on_prompt "$context"
elif [[ "$HOOK_EVENT" == "PostToolUse" ]]; then
  dispatch_on_file_edit "$context"
fi
