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
// License for the specific language governing permissions and limitations under
// the License.

// Package hooks extracts the runtime hook scripts to ~/.claude/docs-harness/hooks/
// and registers them in ~/.claude/settings.json. The scripts are embedded in
// the binary so the installer is self-contained.
package hooks

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/elastic/docs-harness/internal/config"
	"github.com/elastic/docs-harness/internal/prompt"
)

//go:embed scripts/check-updates.sh
var checkUpdatesScript []byte

//go:embed scripts/dispatch.sh
var dispatchScript []byte

//go:embed scripts/dispatch-rules.json
var dispatchRulesJSON []byte

// HooksDir returns the stable directory where hook scripts are extracted.
func HooksDir() (string, error) {
	claudeDir, err := config.ClaudeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(claudeDir, "docs-harness", "hooks"), nil
}

// Extract writes the hook scripts to ~/.claude/docs-harness/hooks/.
// Existing scripts are overwritten so they stay in sync with the installed binary.
func Extract(dryRun bool) error {
	dir, err := HooksDir()
	if err != nil {
		return err
	}

	if dryRun {
		prompt.Info("[dry-run] would extract hook scripts to %s", dir)
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating hooks dir %s: %w", dir, err)
	}

	scripts := map[string][]byte{
		"check-updates.sh":    checkUpdatesScript,
		"dispatch.sh":         dispatchScript,
		"dispatch-rules.json": dispatchRulesJSON,
	}

	for name, content := range scripts {
		path := filepath.Join(dir, name)
		mode := os.FileMode(0o644)
		if filepath.Ext(name) == ".sh" {
			mode = 0o755
		}
		if err := os.WriteFile(path, content, mode); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}
	prompt.OK("Hook scripts extracted to %s", dir)
	return nil
}

// RegisterInSettings appends the harness hook entries into ~/.claude/settings.json.
//
// Claude Code's current hook schema: each event maps to an array of group
// objects with the shape {matcher, hooks: [{type, command}]}.
//
// Re-running is safe: existing entries are never overwritten. Old-format entries
// (flat {type, command, matcher?} from a previous installer version) are removed
// and re-written in the new nested format automatically.
func RegisterInSettings(dryRun bool) error {
	claudeDir, err := config.ClaudeDir()
	if err != nil {
		return err
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	hooksDir := filepath.Join(home, ".claude", "docs-harness", "hooks")
	if runtime.GOOS == "windows" {
		hooksDir = filepath.ToSlash(hooksDir)
	}

	entries := []struct {
		event   string
		matcher string
		command string
	}{
		{"SessionStart", "", fmt.Sprintf(`bash "%s/check-updates.sh" 2>/dev/null || true`, hooksDir)},
		{"UserPromptSubmit", "", fmt.Sprintf(`HOOK_EVENT=UserPromptSubmit bash "%s/dispatch.sh" 2>/dev/null || true`, hooksDir)},
		{"PostToolUse", "Edit|Write|NotebookEdit", fmt.Sprintf(`HOOK_EVENT=PostToolUse bash "%s/dispatch.sh" 2>/dev/null || true`, hooksDir)},
	}

	// Read existing settings — preserve all keys.
	root := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		_ = json.Unmarshal(data, &root)
	}

	hooksMap, _ := root["hooks"].(map[string]any)
	if hooksMap == nil {
		hooksMap = make(map[string]any)
	}

	added := 0
	migrated := 0
	for _, e := range entries {
		existing, _ := hooksMap[e.event].([]any)

		// Remove stale old-format entries for our command.
		// Old format: {type, command, matcher?} at the top level of each array entry.
		// New format: {matcher, hooks: [{type, command}]}
		var cleaned []any
		for _, raw := range existing {
			g, ok := raw.(map[string]any)
			if ok && g["command"] == e.command {
				migrated++
				continue // drop; will re-add in new format below
			}
			cleaned = append(cleaned, raw)
		}
		existing = cleaned

		// Check if our command is already present in any group's nested hooks array.
		alreadyPresent := false
		for _, raw := range existing {
			g, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			inner, _ := g["hooks"].([]any)
			for _, ih := range inner {
				if h, ok := ih.(map[string]any); ok && h["command"] == e.command {
					alreadyPresent = true
					break
				}
			}
			if alreadyPresent {
				break
			}
		}
		if alreadyPresent {
			hooksMap[e.event] = existing
			continue
		}

		// Append a new group: {matcher, hooks: [{type, command}]}
		group := map[string]any{
			"matcher": e.matcher,
			"hooks":   []any{map[string]any{"type": "command", "command": e.command}},
		}
		hooksMap[e.event] = append(existing, group)
		added++
	}

	root["hooks"] = hooksMap

	if dryRun {
		prompt.Info("[dry-run] would append %d hook entry/entries to settings.json (migrate %d)", added, migrated)
		return nil
	}
	if added == 0 && migrated == 0 {
		prompt.OK("Hooks already registered in ~/.claude/settings.json")
		return nil
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return fmt.Errorf("writing settings.json: %w", err)
	}
	switch {
	case migrated > 0 && added > 0:
		prompt.OK("Hooks registered in ~/.claude/settings.json (%d added, %d migrated to new format)", added, migrated)
	case migrated > 0:
		prompt.OK("Hooks migrated to new format in ~/.claude/settings.json (%d entries)", migrated)
	default:
		prompt.OK("Hooks registered in ~/.claude/settings.json (%d added)", added)
	}
	return nil
}
