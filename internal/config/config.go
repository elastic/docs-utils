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

// Package config manages the harness config file (~/.claude/docs-harness.json)
// and provides safe read/merge helpers for Claude Code JSON config files.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// HarnessConfig is the single source of truth stored in ~/.claude/docs-harness.json.
// The plugin hooks and commands read this file at runtime.
type HarnessConfig struct {
	// Provider is "claude" (default) or "litellm".
	Provider string `json:"provider"`
	// LiteLLMBaseURL is the gateway base URL (only set when Provider == "litellm").
	LiteLLMBaseURL string `json:"litellmBaseUrl,omitempty"`
	// Dispatch controls the soft skill-suggestion hooks.
	Dispatch DispatchConfig `json:"dispatch"`
	// Catalogs lists which skill marketplaces are registered ("public", "internal").
	Catalogs []string `json:"catalogs"`
	// UpdateCheck enables the per-session version check hook.
	UpdateCheck bool `json:"updateCheck"`
	// Telemetry enables OTEL_RESOURCE_ATTRIBUTES + OTEL_LOG_TOOL_DETAILS tagging.
	Telemetry bool `json:"telemetry"`
	// HarnessVersion is stamped by the installer for the harness.version resource attr.
	HarnessVersion string `json:"harnessVersion,omitempty"`
}

// DispatchConfig controls the soft dispatch hooks.
type DispatchConfig struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"` // "soft" (only supported mode for now)
}

// DefaultConfig returns sensible defaults for a new installation.
func DefaultConfig(version string) HarnessConfig {
	return HarnessConfig{
		Provider: "claude",
		Dispatch: DispatchConfig{
			Enabled: true,
			Mode:    "soft",
		},
		Catalogs:       []string{"public", "internal"},
		UpdateCheck:    true,
		Telemetry:      true,
		HarnessVersion: version,
	}
}

// ClaudeDir returns the path to ~/.claude/.
func ClaudeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// HarnessConfigPath returns the path to the harness config file.
func HarnessConfigPath() (string, error) {
	dir, err := ClaudeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "docs-harness.json"), nil
}

// Load reads and parses the harness config. Returns the default config if the
// file does not yet exist.
func Load(version string) (HarnessConfig, error) {
	path, err := HarnessConfigPath()
	if err != nil {
		return DefaultConfig(version), err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(version), nil
	}
	if err != nil {
		return DefaultConfig(version), err
	}
	var cfg HarnessConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(version), fmt.Errorf("invalid harness config: %w", err)
	}
	return cfg, nil
}

// Save writes the harness config to disk.
func Save(cfg HarnessConfig, dryRun bool) error {
	path, err := HarnessConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}

// MergeJSON reads a JSON file as a map, applies the provided updates (which
// may be nested), and writes it back — preserving all pre-existing keys.
// If the file does not exist it is created. dryRun skips the write.
func MergeJSON(path string, updates map[string]any, dryRun bool) error {
	existing := make(map[string]any)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	deepMerge(existing, updates)
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// RemoveEnvKeys reads settings.json and removes the specified keys from the
// `env` map, preserving all other top-level keys. No-op if the file or key
// doesn't exist.
func RemoveEnvKeys(settingsPath string, keys []string, dryRun bool) error {
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parsing settings.json: %w", err)
	}
	env, _ := root["env"].(map[string]any)
	if env == nil {
		return nil
	}
	for _, k := range keys {
		delete(env, k)
	}
	if len(env) == 0 {
		delete(root, "env")
	} else {
		root["env"] = env
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	return os.WriteFile(settingsPath, out, 0o644)
}

// deepMerge merges src into dst. Map values are merged recursively;
// all other values in src overwrite dst.
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		if sm, ok := sv.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				deepMerge(dm, sm)
				continue
			}
		}
		dst[k] = sv
	}
}
