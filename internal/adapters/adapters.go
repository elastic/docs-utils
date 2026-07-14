// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Apache License,
// Version 2.0 (the "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// Package adapters configures the small, host-specific layer around the
// shared Elastic Docs skills directory.
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/elastic/docs-utils/internal/hosts"
	"github.com/elastic/docs-utils/internal/state"
)

const (
	publicMCP       = "https://www.elastic.co/docs/_mcp/"
	legacyPublicMCP = "https://d34ipnu52o64md.cloudfront.net/docs/_mcp"
	internalMCP     = "https://codex.elastic.dev/mcp"
)

type Result struct {
	Hosts     map[string]state.HostState
	Validated []string
	Warnings  []string
}

// MigrateLegacy removes only the hook commands that belonged to the retired
// docs-harness prototype. It deliberately leaves provider and other Claude
// settings untouched.
func MigrateLegacy(dryRun bool) (bool, error) {
	path, err := claudeHookPath()
	if err != nil {
		return false, err
	}
	root, err := readObject(path)
	if err != nil {
		return false, err
	}
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return false, nil
	}
	changed := false
	for event, raw := range hooks {
		groups, ok := raw.([]any)
		if !ok {
			continue
		}
		keptGroups := make([]any, 0, len(groups))
		for _, rawGroup := range groups {
			group, ok := rawGroup.(map[string]any)
			if !ok {
				keptGroups = append(keptGroups, rawGroup)
				continue
			}
			entries, ok := group["hooks"].([]any)
			if !ok {
				keptGroups = append(keptGroups, rawGroup)
				continue
			}
			keptEntries := make([]any, 0, len(entries))
			for _, rawEntry := range entries {
				entry, ok := rawEntry.(map[string]any)
				command, _ := entry["command"].(string)
				if ok && strings.Contains(command, ".claude/docs-harness/hooks/") {
					changed = true
					continue
				}
				keptEntries = append(keptEntries, rawEntry)
			}
			if len(keptEntries) > 0 {
				group["hooks"] = keptEntries
				keptGroups = append(keptGroups, group)
			}
		}
		if len(keptGroups) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = keptGroups
		}
	}
	if !changed || dryRun {
		return changed, nil
	}
	root["hooks"] = hooks
	return true, writeObject(path, root)
}

// Sync configures MCP entries in supported hosts.
func Sync(targets []hosts.ID, internal, dryRun, force bool) (Result, error) {
	result := Result{Hosts: map[string]state.HostState{}}
	for _, host := range targets {
		var files []string
		var err error
		switch host {
		case hosts.Claude:
			files, err = addClaude(internal, dryRun)
		case hosts.Codex:
			files, err = addCodex(internal, dryRun)
		case hosts.Cursor:
			files, err = writeCursor(internal, dryRun, force)
		case hosts.OpenCode:
			files, err = writeOpenCode(internal, dryRun, force)
		}
		if err != nil {
			return result, fmt.Errorf("configure %s: %w", host, err)
		}
		result.Hosts[string(host)] = state.HostState{Files: files}
		if !dryRun {
			if err := validate(host, internal, files); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s MCP validation: %v", host, err))
			} else {
				result.Validated = append(result.Validated, string(host))
			}
		}
	}
	return result, nil
}

// validate asks each host to list its MCP configuration after setup. A failed
// validation is a warning rather than a rollback: the config may be present
// while the host is offline or unable to reach a remote MCP server.
func validate(host hosts.ID, internal bool, files []string) error {
	command, args := "", []string(nil)
	switch host {
	case hosts.Claude:
		command, args = "claude", []string{"mcp", "list"}
	case hosts.Codex:
		command, args = "codex", []string{"mcp", "list"}
	case hosts.Cursor:
		command, args = "agent", []string{"mcp", "list"}
	case hosts.OpenCode:
		command, args = "opencode", []string{"mcp", "list"}
	default:
		return fmt.Errorf("no MCP validator for %s", host)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("%s mcp list timed out", command)
	}
	if err != nil {
		return fmt.Errorf("%s mcp list: %s", command, strings.TrimSpace(string(out)))
	}
	if err := validateListOutput(string(out), internal); err != nil {
		return fmt.Errorf("%s mcp list: %w", command, err)
	}
	return nil
}

func validateListOutput(output string, internal bool) error {
	for _, server := range servers(internal) {
		if !strings.Contains(output, server.name) {
			return fmt.Errorf("did not include %q", server.name)
		}
	}
	return nil
}

func servers(internal bool) []struct{ name, url string } {
	items := []struct{ name, url string }{{"elastic-docs", publicMCP}}
	if internal {
		items = append(items, struct{ name, url string }{"elastic-internal-docs", internalMCP})
	}
	return items
}

func addClaude(internal, dryRun bool) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	for _, server := range servers(internal) {
		if dryRun {
			continue
		}
		if err := runAdd("claude", "mcp", "add", "--scope", "user", "--transport", "http", server.name, server.url); err != nil {
			return nil, err
		}
	}
	path := filepath.Join(home, ".claude", "settings.json")
	if dryRun {
		return []string{filepath.Join(home, ".claude.json"), path}, nil
	}
	if err := writeClaudeHook(path); err != nil {
		return nil, err
	}
	return []string{filepath.Join(home, ".claude.json"), path}, nil
}

func addCodex(internal, dryRun bool) ([]string, error) {
	for _, server := range servers(internal) {
		if dryRun {
			continue
		}
		if err := runAdd("codex", "mcp", "add", server.name, "--url", server.url); err != nil {
			return nil, err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return []string{filepath.Join(home, ".codex", "config.toml")}, nil
}

func runAdd(command string, args ...string) error {
	out, err := exec.Command(command, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.ToLower(string(out))
	if strings.Contains(message, "already exists") || strings.Contains(message, "already configured") || strings.Contains(message, "exists") {
		return nil
	}
	return fmt.Errorf("%s %s: %s", command, strings.Join(args, " "), strings.TrimSpace(string(out)))
}

func writeCursor(internal, dryRun, force bool) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".cursor", "mcp.json")
	if dryRun {
		return []string{path}, nil
	}
	root, err := readObject(path)
	if err != nil {
		return nil, err
	}
	mcp := object(root, "mcpServers")
	for _, server := range servers(internal) {
		if err := mergeServer(mcp, server.name, map[string]any{"url": server.url}, force); err != nil {
			return nil, err
		}
	}
	root["mcpServers"] = mcp
	return []string{path}, writeObject(path, root)
}

func writeOpenCode(internal, dryRun, force bool) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path, err := openCodeConfigPath(home)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return []string{path}, nil
	}
	root, err := readObject(path)
	if err != nil {
		return nil, err
	}
	mcp := object(root, "mcp")
	for _, server := range servers(internal) {
		if err := mergeServer(mcp, server.name, map[string]any{"type": "remote", "url": server.url, "enabled": true}, force); err != nil {
			return nil, err
		}
	}
	root["mcp"] = mcp
	if err := writeObject(path, root); err != nil {
		return nil, err
	}
	if err := cleanupLegacyOpenCodeConfig(path); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func openCodeConfigPath(home string) (string, error) {
	if runtime.GOOS != "windows" {
		return filepath.Join(home, ".config", "opencode", "opencode.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "opencode", "opencode.json"), nil
}

// cleanupLegacyOpenCodeConfig removes only this tool's known MCP entries from
// the macOS path used by an early development build. OpenCode itself reads the
// XDG-style path returned above.
func cleanupLegacyOpenCodeConfig(canonicalPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	legacyPath := filepath.Join(dir, "opencode", "opencode.json")
	if legacyPath == canonicalPath {
		return nil
	}
	if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	root, err := readObject(legacyPath)
	if err != nil {
		return err
	}
	mcp, ok := root["mcp"].(map[string]any)
	if !ok {
		return nil
	}
	changed := false
	for _, name := range []string{"elastic-docs", "elastic-internal-docs"} {
		if current, ok := mcp[name].(map[string]any); ok && validServerURL(name, current["url"]) {
			delete(mcp, name)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if len(mcp) == 0 {
		delete(root, "mcp")
	} else {
		root["mcp"] = mcp
	}
	return writeObject(legacyPath, root)
}

func claudeHookPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func writeClaudeHook(path string) error {
	root, err := readObject(path)
	if err != nil {
		return err
	}
	hooks := object(root, "hooks")
	const command = "elastic-docs-utils --color=never hook session-start --host claude"
	if existing, ok := hooks["SessionStart"].([]any); ok {
		for _, item := range existing {
			group, ok := item.(map[string]any)
			if !ok {
				continue
			}
			hookItems, _ := group["hooks"].([]any)
			for _, hook := range hookItems {
				if entry, ok := hook.(map[string]any); ok && entry["command"] == command {
					return nil
				}
			}
		}
	}
	entry := map[string]any{"type": "command", "command": command}
	group := map[string]any{"hooks": []any{entry}}
	current, _ := hooks["SessionStart"].([]any)
	hooks["SessionStart"] = append(current, group)
	root["hooks"] = hooks
	return writeObject(path, root)
}

func readObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return root, nil
}

func object(root map[string]any, key string) map[string]any {
	if value, ok := root[key].(map[string]any); ok {
		return value
	}
	return map[string]any{}
}

func mergeServer(servers map[string]any, name string, wanted map[string]any, force bool) error {
	if existing, ok := servers[name]; ok {
		if current, ok := existing.(map[string]any); ok && validServerURL(name, current["url"]) {
			return nil
		}
		if !force {
			return fmt.Errorf("MCP server %q already has a different configuration; rerun with --force to replace it", name)
		}
	}
	servers[name] = wanted
	return nil
}

func validServerURL(name string, value any) bool {
	url, ok := value.(string)
	if !ok {
		return false
	}
	switch name {
	case "elastic-docs":
		return url == publicMCP || url == legacyPublicMCP
	case "elastic-internal-docs":
		return url == internalMCP
	default:
		return false
	}
}

func writeObject(path string, root map[string]any) error {
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".elastic-docs-utils-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
