// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright ownership.
// Elasticsearch B.V. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations under
// the License.

// Package telemetry configures Claude Code's native OpenTelemetry support for
// the docs harness. We do NOT own the endpoint (the org remote-settings.json
// already streams to otel.zocalo.inf.elasticnet.co). We only:
//   - Add OTEL_RESOURCE_ATTRIBUTES to tag usage as team=docs in the existing pipeline.
//   - Set OTEL_LOG_TOOL_DETAILS=1 so MCP server/tool and skill names are populated
//     (without it they collapse to generic labels in the metrics attributes).
//
// Both are written into settings.json `env` so they apply to every session.
// The endpoint, headers, and exporters are left untouched.
package telemetry

import (
	"fmt"
	"path/filepath"

	"github.com/elastic/docs-harness/internal/config"
	"github.com/elastic/docs-harness/internal/prompt"
)

// Apply writes the telemetry tagging config into settings.json.
// version is stamped into the harness.version resource attribute.
func Apply(version string, dryRun bool) error {
	claudeDir, err := config.ClaudeDir()
	if err != nil {
		return err
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	resourceAttrs := fmt.Sprintf("team=docs,harness=docs-harness,harness.version=%s", version)
	updates := map[string]any{
		"env": map[string]any{
			"OTEL_LOG_TOOL_DETAILS":    "1",
			"OTEL_RESOURCE_ATTRIBUTES": resourceAttrs,
		},
	}
	if err := config.MergeJSON(settingsPath, updates, dryRun); err != nil {
		return fmt.Errorf("updating settings.json: %w", err)
	}
	prompt.OK("Telemetry tags applied")
	return nil
}

// Remove strips the harness telemetry keys from settings.json `env`.
func Remove(dryRun bool) error {
	claudeDir, err := config.ClaudeDir()
	if err != nil {
		return err
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	keys := []string{"OTEL_LOG_TOOL_DETAILS", "OTEL_RESOURCE_ATTRIBUTES"}
	if err := config.RemoveEnvKeys(settingsPath, keys, dryRun); err != nil {
		return fmt.Errorf("updating settings.json: %w", err)
	}
	prompt.OK("Telemetry tags removed from settings.json")
	return nil
}
