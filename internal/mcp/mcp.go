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

// Package mcp ensures the Elastic Docs MCP server entries exist in ~/.claude.json.
// The plugin's .mcp.json provides the servers to Claude Code when the plugin is
// active; this package adds them to the global ~/.claude.json as a belt-and-
// suspenders measure for users who haven't enabled the plugin globally.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elastic/docs-harness/internal/prompt"
)

// Server describes an MCP server entry.
type Server struct {
	Name string
	Type string
	URL  string
}

// Required returns the two Elastic docs MCP servers the harness needs.
func Required() []Server {
	return []Server{
		{
			Name: "elastic-internal-docs",
			Type: "http",
			URL:  "https://codex.elastic.dev/mcp",
		},
		{
			Name: "elastic-docs",
			Type: "http",
			URL:  "https://www.elastic.co/docs/_mcp/",
		},
	}
}

// EnsureGlobal ensures both MCP servers exist in ~/.claude.json.
// Existing entries with differing URLs are not overwritten unless force is true.
func EnsureGlobal(force, dryRun bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot find home directory: %w", err)
	}
	claudeJSON := filepath.Join(home, ".claude.json")

	raw := make(map[string]any)
	if data, err := os.ReadFile(claudeJSON); err == nil {
		_ = json.Unmarshal(data, &raw)
	}

	// Ensure mcpServers key exists.
	mcpServers, _ := raw["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = make(map[string]any)
	}

	changed := false
	for _, s := range Required() {
		existing, exists := mcpServers[s.Name]
		if exists && !force {
			// Verify it matches; warn if it doesn't but don't overwrite.
			if em, ok := existing.(map[string]any); ok {
				if em["url"] != s.URL {
					prompt.Warn("MCP server %q already configured with a different URL — skipping (use --force to overwrite)", s.Name)
					continue
				}
			}
			prompt.OK("MCP server already configured: %s", s.Name)
			continue
		}
		mcpServers[s.Name] = map[string]any{
			"type": s.Type,
			"url":  s.URL,
		}
		changed = true
		prompt.OK("MCP server configured: %s → %s", s.Name, s.URL)
	}

	if !changed {
		return nil
	}
	raw["mcpServers"] = mcpServers

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	return os.WriteFile(claudeJSON, out, 0o644)
}
