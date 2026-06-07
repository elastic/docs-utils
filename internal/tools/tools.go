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

// Package tools checks installed versions of docs-builder and Vale against
// the latest GitHub releases and offers upgrades. Never auto-installs.
package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/elastic/docs-harness/internal/prompt"
)

// ToolStatus holds the result of a version check for one tool.
type ToolStatus struct {
	Name      string
	Installed string // empty if not found
	Latest    string // empty if check failed
	Behind    bool
	UpgradeHint string
}

// CheckAll checks docs-builder, the Elastic Vale rules, and elastic-docs-skills.
func CheckAll() []ToolStatus {
	results := make(chan ToolStatus, 3)
	go func() { results <- checkDocsBuilder() }()
	go func() { results <- checkValeRules() }()
	go func() { results <- checkElasticDocsSkills() }()

	statuses := make([]ToolStatus, 0, 3)
	for i := 0; i < 3; i++ {
		statuses = append(statuses, <-results)
	}
	return statuses
}

// PrintSummary prints a one-line notice per tool that is behind, silent if current.
func PrintSummary(statuses []ToolStatus) {
	anyBehind := false
	for _, s := range statuses {
		if s.Behind {
			anyBehind = true
			prompt.Warn("%s %s → %s available — run /docs-update or `%s`",
				s.Name, s.Installed, s.Latest, s.UpgradeHint)
		}
	}
	if !anyBehind {
		// Silent — do not spam every session.
	}
}

// checkDocsBuilder checks elastic/docs-builder.
func checkDocsBuilder() ToolStatus {
	s := ToolStatus{Name: "docs-builder"}

	path, err := exec.LookPath("docs-builder")
	if err != nil {
		s.Installed = "(not found)"
		return s
	}

	out, err := exec.Command(path).Output()
	if err == nil {
		s.Installed = parseVersion(string(out))
	}

	latest, err := latestGitHubRelease("elastic", "docs-builder")
	if err != nil {
		return s
	}
	s.Latest = latest
	s.Behind = semverGT(latest, s.Installed)
	if runtime.GOOS == "windows" {
		s.UpgradeHint = `iex (New-Object System.Net.WebClient).DownloadString('https://ela.st/docs-builder-install-win')`
	} else {
		s.UpgradeHint = "curl -sL https://ela.st/docs-builder-install | sh"
	}
	return s
}

// checkValeRules checks the Elastic Vale rules package (elastic/vale-rules),
// not the vale binary version. The installed version is read from
// styles/Elastic/VERSION, written by the vale-rules release workflow.
func checkValeRules() ToolStatus {
	s := ToolStatus{Name: "elastic-vale-rules"}

	installed, _ := installedValeRulesVersion()
	if installed == "" {
		s.Installed = "(not installed)"
		s.UpgradeHint = valeRulesInstallHint()
		return s
	}
	s.Installed = installed

	latest, err := latestGitHubRelease("elastic", "vale-rules")
	if err != nil {
		return s
	}
	s.Latest = latest
	s.Behind = semverGT(latest, s.Installed)
	s.UpgradeHint = valeRulesInstallHint()
	return s
}

func valeRulesInstallHint() string {
	switch runtime.GOOS {
	case "windows":
		return "powershell -ExecutionPolicy Bypass -File install-windows.ps1  (see /docs-update)"
	case "linux":
		return "curl -fsSL https://raw.githubusercontent.com/elastic/vale-rules/main/install-linux.sh | bash"
	default: // darwin
		return "curl -fsSL https://raw.githubusercontent.com/elastic/vale-rules/main/install-macos.sh | bash"
	}
}

// installedValeRulesVersion reads the VERSION file written by the vale-rules
// release into the Elastic styles directory. Returns the version string and
// the parent directory, or empty strings if not found.
func installedValeRulesVersion() (version, dir string) {
	candidates := valeStylesCandidates()
	for _, base := range candidates {
		versionFile := filepath.Join(base, "Elastic", "VERSION")
		data, err := os.ReadFile(versionFile)
		if err == nil {
			return strings.TrimSpace(string(data)), base
		}
	}
	return "", ""
}

// valeStylesCandidates returns directories to search for the Elastic/ styles
// folder, in priority order.
func valeStylesCandidates() []string {
	var dirs []string

	// 1. Explicit env override (e.g. VALE_STYLES_PATH in .zshrc).
	if v := os.Getenv("VALE_STYLES_PATH"); v != "" {
		dirs = append(dirs, v)
	}

	// 2. Ask vale itself where it stores styles.
	if path, err := exec.LookPath("vale"); err == nil {
		if out, err := exec.Command(path, "ls-dirs").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "StylesPath") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						dirs = append(dirs, strings.TrimSpace(parts[1]))
					}
				}
			}
		}
	}

	// 3. Platform defaults.
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		dirs = append(dirs,
			filepath.Join(home, "Library", "Application Support", "vale", "styles"),
		)
	case "linux":
		dirs = append(dirs,
			filepath.Join(home, ".local", "share", "vale", "styles"),
			filepath.Join(home, ".vale", "styles"),
		)
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			dirs = append(dirs, filepath.Join(appdata, "vale", "styles"))
		}
	}
	return dirs
}

var versionRE = regexp.MustCompile(`\d+\.\d+\.\d+`)

func parseVersion(s string) string {
	// docs-builder includes a git hash suffix: "1.16.1+947c578…"; strip after +
	s = strings.SplitN(s, "+", 2)[0]
	m := versionRE.FindString(s)
	return strings.TrimSpace(m)
}

// checkElasticDocsSkills compares the installed plugin version (from
// ~/.claude/plugins/installed_plugins.json) against the version field in
// the plugin.json on the main branch of elastic/elastic-docs-skills.
func checkElasticDocsSkills() ToolStatus {
	s := ToolStatus{Name: "elastic-docs-skills"}

	installed := installedPluginVersion("elastic-docs-skills@elastic-docs-skills")
	if installed == "" {
		s.Installed = "(not installed)"
		s.UpgradeHint = "claude plugin install elastic-docs-skills@elastic-docs-skills --scope user"
		return s
	}
	s.Installed = installed

	latest, err := latestPluginJSONVersion("elastic", "elastic-docs-skills")
	if err != nil {
		return s
	}
	s.Latest = latest
	s.Behind = semverGT(latest, s.Installed)
	s.UpgradeHint = "claude plugin marketplace update && claude plugin update elastic-docs-skills@elastic-docs-skills --scope user"
	return s
}

// installedPluginVersion reads the version of a named plugin from
// ~/.claude/plugins/installed_plugins.json.
func installedPluginVersion(pluginKey string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var f struct {
		Plugins map[string][]struct {
			Version string `json:"version"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return ""
	}
	entries := f.Plugins[pluginKey]
	if len(entries) == 0 {
		return ""
	}
	return entries[0].Version
}

// latestPluginJSONVersion fetches the version field from plugin.json on the
// default branch of a GitHub repo.
func latestPluginJSONVersion(owner, repo string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/.claude-plugin/plugin.json", owner, repo)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("fetching plugin.json: HTTP %d", resp.StatusCode)
	}
	var p struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return "", err
	}
	return parseVersion(p.Version), nil
}

// latestGitHubRelease fetches the latest release tag from the GitHub API.
func latestGitHubRelease(owner, repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var result struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return parseVersion(result.TagName), nil
}

// semverGT returns true if a > b in semver ordering.
func semverGT(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	pa := parseSemver(a)
	pb := parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return true
		}
		if pa[i] < pb[i] {
			return false
		}
	}
	return false
}

func parseSemver(s string) [3]int {
	var v [3]int
	parts := strings.SplitN(strings.TrimLeft(s, "v"), ".", 3)
	for i := 0; i < 3 && i < len(parts); i++ {
		fmt.Sscanf(parts[i], "%d", &v[i])
	}
	return v
}
