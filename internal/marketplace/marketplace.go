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

// Package marketplace registers Claude Code plugin marketplaces and installs plugins.
// Primary path: shells out to the `claude` CLI.
// Fallback: merges directly into ~/.claude/plugins/known_marketplaces.json and settings.json.
package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/elastic/docs-harness/internal/config"
	"github.com/elastic/docs-harness/internal/prompt"
)

// Catalog describes a marketplace + its plugin name.
type Catalog struct {
	// MarketplaceName is the short name used with `claude plugin marketplace add`.
	MarketplaceName string
	// GitHubRepo is the "owner/repo" path on GitHub (no scheme, no .git suffix).
	GitHubRepo string
	// Private marks repos that require auth to clone; SSH is preferred for these.
	Private bool
	// PluginName is the `<plugin>@<marketplace>` identifier for `claude plugin install`.
	PluginName string
	// Label is a human-readable name for display.
	Label string
}

// gitURL returns the appropriate source for this catalog.
// Private repos use SSH when SSH auth is available; otherwise HTTPS.
// For the docs-harness catalog, falls back to the local clone path if the
// repo is not yet reachable on GitHub (e.g. pre-publication).
func (c Catalog) gitURL() string {
	if c.Private && sshAvailableForGitHub() {
		return "git@github.com:" + c.GitHubRepo + ".git"
	}
	return "https://github.com/" + c.GitHubRepo + ".git"
}

// localFallbackURL returns the local repo path for the docs-harness marketplace
// when the binary is running from a clone. Used only when GitHub registration fails.
func localFallbackURL() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	candidates := []string{filepath.Dir(dir), dir}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, ".claude-plugin", "plugin.json")); err == nil {
			return candidate
		}
	}
	return ""
}

// localRepoPath returns the docs-harness repo root if the running binary is
// inside a clone of it (i.e. .claude-plugin/plugin.json exists at the root).
// Returns "" if not running from a local clone.
func localRepoPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	// Binary lives at <repo>/bin/docs-harness_* — walk up to find the root.
	dir := filepath.Dir(exe) // <repo>/bin
	candidates := []string{
		filepath.Dir(dir),        // <repo>         (binary in bin/)
		dir,                      // <repo>         (binary in root)
		filepath.Dir(filepath.Dir(dir)), // grandparent  (fallback)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, ".claude-plugin", "plugin.json")); err == nil {
			return candidate
		}
	}
	return ""
}

// All returns the full catalog list. Callers filter by user's catalog selection.
func All() []Catalog {
	return []Catalog{
		{
			MarketplaceName: "docs-harness",
			GitHubRepo:      "elastic/docs-harness",
			Private:         false,
			PluginName:      "docs-harness@docs-harness",
			Label:           "Elastic Docs Harness",
		},
		{
			MarketplaceName: "elastic-docs-skills",
			GitHubRepo:      "elastic/elastic-docs-skills",
			Private:         false,
			PluginName:      "elastic-docs-skills@elastic-docs-skills",
			Label:           "Elastic Docs Skills (public)",
		},
	}
}

// sshCheck caches the result of the SSH availability probe.
var sshCheck struct {
	sync.Once
	available bool
}

// sshAvailableForGitHub returns true if `ssh -T git@github.com` succeeds
// (exit 1 with "successfully authenticated" is GitHub's normal success response).
// The probe runs at most once per process and times out after 5 seconds.
func sshAvailableForGitHub() bool {
	sshCheck.Do(func() {
		cmd := exec.Command("ssh", "-T", "-o", "StrictHostKeyChecking=no",
			"-o", "BatchMode=yes",
			"-o", "ConnectTimeout=5",
			"git@github.com")
		out, _ := cmd.CombinedOutput()
		// GitHub exits 1 but prints "successfully authenticated" on success.
		sshCheck.available = strings.Contains(string(out), "successfully authenticated")
	})
	return sshCheck.available
}

// hasCLI returns true if the `claude` binary is on PATH.
func hasCLI() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// EnsureAll registers marketplaces and installs plugins for the given catalogs.
// catalogs is the user's selection slice from HarnessConfig (e.g. ["public","internal"]).
func EnsureAll(catalogs []string, dryRun bool) error {
	selected := selectCatalogs(catalogs)
	if hasCLI() {
		return ensureViaCLI(selected, dryRun)
	}
	prompt.Warn("claude CLI not found — falling back to direct JSON config merge")
	return ensureViaJSON(selected, dryRun)
}

// Update pulls the latest from all registered marketplaces then updates each
// installed plugin. This is the --update path; the full install is EnsureAll.
func Update(dryRun bool) error {
	if !hasCLI() {
		return fmt.Errorf("claude CLI not found; run the installer to update")
	}

	// 1. Refresh marketplace metadata.
	if dryRun {
		prompt.Info("[dry-run] would run: claude plugin marketplace update")
	} else {
		cmd := exec.Command("claude", "plugin", "marketplace", "update")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			prompt.Warn("marketplace update failed: %v", err)
		}
	}

	// 2. Update each plugin.
	for _, c := range All() {
		if dryRun {
			prompt.Info("[dry-run] would run: claude plugin update %s --scope user", c.PluginName)
			continue
		}
		out, err := exec.Command("claude", "plugin", "update", c.PluginName, "--scope", "user").CombinedOutput()
		outStr := strings.ToLower(string(out))
		if err != nil {
			if strings.Contains(outStr, "already") || strings.Contains(outStr, "up to date") {
				prompt.OK("%s is already up to date", c.Label)
			} else {
				prompt.Warn("Could not update %s: %s", c.Label, strings.TrimSpace(string(out)))
			}
		} else {
			prompt.OK("Updated: %s", c.Label)
		}
	}
	return nil
}

func selectCatalogs(_ []string) []Catalog {
	return All()
}

// upgradeLocalToRemote detects whether the docs-harness marketplace was
// previously registered with a local path (e.g. during private-repo phase)
// and, if the GitHub URL is now reachable, swaps the source to the canonical
// remote URL so subsequent installs and updates use GitHub.
func upgradeLocalToRemote(dryRun bool) {
	claudeDir, err := config.ClaudeDir()
	if err != nil {
		return
	}
	knownPath := filepath.Join(claudeDir, "plugins", "known_marketplaces.json")

	data, err := os.ReadFile(knownPath)
	if err != nil {
		return // file doesn't exist yet — nothing to upgrade
	}

	var known map[string]any
	if err := json.Unmarshal(data, &known); err != nil {
		return
	}

	entry, ok := known["docs-harness"].(map[string]any)
	if !ok {
		return
	}

	source, _ := entry["source"].(string)
	if source == "" {
		return
	}

	// Already remote — nothing to do.
	if strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "git@") {
		return
	}

	// Source is a local path. Build the preferred remote URL.
	remoteURL := "https://github.com/elastic/docs-harness.git"
	if sshAvailableForGitHub() {
		remoteURL = "git@github.com:elastic/docs-harness.git"
	}

	if dryRun {
		prompt.Info("[dry-run] would upgrade docs-harness marketplace from local path to %s", remoteURL)
		return
	}

	// Best-effort CLI removal so the CLI's own state is also cleared.
	if hasCLI() {
		_ = exec.Command("claude", "plugin", "marketplace", "remove", "docs-harness").Run()
	}

	// Update the JSON directly.
	entry["source"] = remoteURL
	known["docs-harness"] = entry

	out, err := json.MarshalIndent(known, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(knownPath, out, 0o644); err != nil {
		return
	}
	prompt.OK("Upgraded docs-harness marketplace: local path → %s", remoteURL)
}

// ensureViaCLI uses `claude plugin marketplace add` and `claude plugin install`.
func ensureViaCLI(catalogs []Catalog, dryRun bool) error {
	// 0. For docs-harness: if previously registered from a local path but the
	//    GitHub URL is now reachable, remove the stale local registration first
	//    so the next add picks up the canonical remote URL.
	upgradeLocalToRemote(dryRun)

	// 1. Add each marketplace (idempotent — CLI skips existing ones).
	for _, c := range catalogs {
		url := c.gitURL()
		if c.Private {
			if sshAvailableForGitHub() {
				prompt.Info("Using SSH for private repo: %s", c.Label)
			} else {
				prompt.Warn("SSH not available for GitHub — using HTTPS for %s (ensure `gh auth login` is done or git credentials are configured)", c.Label)
			}
		}
		if dryRun {
			prompt.Info("[dry-run] would run: claude plugin marketplace add %s", url)
			continue
		}
		out, err := exec.Command("claude", "plugin", "marketplace", "add", url).CombinedOutput()
		outStr := strings.ToLower(string(out))
		if err != nil {
			if strings.Contains(outStr, "already") || strings.Contains(outStr, "exists") {
				prompt.OK("Marketplace already registered: %s", c.Label)
			} else if c.MarketplaceName == "docs-harness" {
				// GitHub URL failed (repo may not be public yet) — try local clone.
				if local := localFallbackURL(); local != "" {
					prompt.Warn("GitHub unavailable for %s — trying local clone: %s", c.Label, local)
					out2, err2 := exec.Command("claude", "plugin", "marketplace", "add", local).CombinedOutput()
					if err2 == nil || strings.Contains(strings.ToLower(string(out2)), "already") {
						prompt.OK("Marketplace registered from local clone: %s", c.Label)
					} else {
						prompt.Warn("Could not register marketplace %s: %s", c.Label, strings.TrimSpace(string(out2)))
					}
				} else {
					prompt.Warn("Could not register marketplace %s: %s", c.Label, strings.TrimSpace(string(out)))
				}
			} else {
				prompt.Warn("Could not register marketplace %s: %s", c.Label, strings.TrimSpace(string(out)))
			}
		} else {
			prompt.OK("Marketplace registered: %s", c.Label)
		}
	}

	// 2. Install / ensure each plugin user-scoped.
	for _, c := range catalogs {
		if dryRun {
			prompt.Info("[dry-run] would run: claude plugin install %s --scope user", c.PluginName)
			continue
		}
		out, err := exec.Command("claude", "plugin", "install", c.PluginName, "--scope", "user").CombinedOutput()
		outStr := strings.ToLower(string(out))
		if err != nil {
			if strings.Contains(outStr, "already") || strings.Contains(outStr, "installed") {
				prompt.OK("Plugin already installed: %s", c.Label)
			} else {
				prompt.Warn("Could not install plugin %s: %s", c.Label, strings.TrimSpace(string(out)))
			}
		} else {
			prompt.OK("Plugin installed: %s", c.Label)
		}
	}
	return nil
}

// ensureViaJSON merges directly into known_marketplaces.json and settings.json.
func ensureViaJSON(catalogs []Catalog, dryRun bool) error {
	claudeDir, err := config.ClaudeDir()
	if err != nil {
		return err
	}

	// known_marketplaces.json — append entries.
	knownPath := filepath.Join(claudeDir, "plugins", "known_marketplaces.json")
	existing := make(map[string]any)
	if data, err := os.ReadFile(knownPath); err == nil {
		_ = json.Unmarshal(data, &existing)
	}
	for _, c := range catalogs {
		if _, ok := existing[c.MarketplaceName]; !ok {
			existing[c.MarketplaceName] = map[string]any{
				"type":   "git",
				"source": c.gitURL(),
			}
		}
	}
	if !dryRun {
		if err := os.MkdirAll(filepath.Dir(knownPath), 0o755); err != nil {
			return err
		}
		data, _ := json.MarshalIndent(existing, "", "  ")
		if err := os.WriteFile(knownPath, data, 0o644); err != nil {
			return fmt.Errorf("writing known_marketplaces.json: %w", err)
		}
	}
	prompt.OK("Updated known_marketplaces.json (%d marketplace(s))", len(catalogs))

	// settings.json — merge extraKnownMarketplaces and enabledPlugins.
	settingsPath := filepath.Join(claudeDir, "settings.json")
	extraMarketplaces := make(map[string]any)
	enabledPlugins := make(map[string]any)
	for _, c := range catalogs {
		extraMarketplaces[c.MarketplaceName] = map[string]any{"source": c.gitURL()}
		enabledPlugins[c.PluginName] = true
	}
	updates := map[string]any{
		"extraKnownMarketplaces": extraMarketplaces,
		"enabledPlugins":         enabledPlugins,
	}
	if err := config.MergeJSON(settingsPath, updates, dryRun); err != nil {
		return fmt.Errorf("updating settings.json: %w", err)
	}
	prompt.OK("Updated settings.json (marketplaces + enabled plugins)")
	return nil
}
