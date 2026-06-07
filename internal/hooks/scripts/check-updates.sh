#!/usr/bin/env bash
# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Apache License, Version 2.0.
#
# SessionStart hook: check docs-builder / Vale / catalog versions.
# Silent when everything is current. Prints a one-line notice per outdated tool.
# Throttled to once per day via a timestamp file.

set -euo pipefail

HARNESS_CONFIG="${HOME}/.claude/docs-harness.json"
TIMESTAMP_FILE="${HOME}/.claude/docs-harness/.last-update-check"
THROTTLE_SECONDS=86400  # 24 hours

# Exit early if update checks are disabled.
if command -v python3 &>/dev/null && [[ -f "$HARNESS_CONFIG" ]]; then
  update_check=$(python3 -c "
import json, sys
try:
    d = json.load(open('$HARNESS_CONFIG'))
    print('true' if d.get('updateCheck', True) else 'false')
except Exception:
    print('true')
")
  [[ "$update_check" == "false" ]] && exit 0
fi

# Throttle: skip if checked recently.
if [[ -f "$TIMESTAMP_FILE" ]]; then
  last=$(cat "$TIMESTAMP_FILE" 2>/dev/null || echo 0)
  now=$(date +%s)
  elapsed=$(( now - last ))
  [[ $elapsed -lt $THROTTLE_SECONDS ]] && exit 0
fi

# Run the version check via the installed binary (fast, parallel checks).
if command -v docs-harness &>/dev/null; then
  docs-harness check-updates
fi

# Stamp the timestamp.
mkdir -p "$(dirname "$TIMESTAMP_FILE")"
date +%s > "$TIMESTAMP_FILE"
