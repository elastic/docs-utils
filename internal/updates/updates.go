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

// Package updates checks documentation tool versions and caches the results.
package updates

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/elastic/docs-utils/internal/paths"
	"github.com/elastic/docs-utils/internal/state"
)

const CacheTTL = 24 * time.Hour

type Item struct {
	Name      string `json:"name"`
	Installed string `json:"installed"`
	Latest    string `json:"latest,omitempty"`
	State     string `json:"state"`
	Hint      string `json:"hint,omitempty"`
}

type Status struct {
	CheckedAt time.Time `json:"checkedAt"`
	Items     []Item    `json:"items"`
}

func Load() (Status, error) {
	path, err := paths.UpdateStatusPath()
	if err != nil {
		return Status{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}, fmt.Errorf("parse update cache: %w", err)
	}
	return status, nil
}

func Stale(status Status, now time.Time) bool {
	return status.CheckedAt.IsZero() || now.Sub(status.CheckedAt) > CacheTTL
}

// Refresh performs bounded network checks and atomically persists the result.
func Refresh(version string) (Status, error) {
	items := []Item{
		checkElasticDocsUtils(version),
		checkDocsBuilder(),
		checkVale(),
		checkValeRules(),
		checkSkills(),
	}
	status := Status{CheckedAt: time.Now().UTC(), Items: items}
	if err := save(status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func checkDocsBuilder() Item {
	installed := binaryVersion("docs-builder", "--version")
	latest := githubRelease("elastic", "docs-builder")
	return compare("docs-builder", installed, latest, "Run `elastic-docs-utils update --component docs-builder`")
}

func checkVale() Item {
	installed := binaryVersion("vale", "--version")
	latest := githubRelease("errata-ai", "vale")
	return compare("Vale", installed, latest, "Update Vale with your package manager")
}

func checkValeRules() Item {
	installed := valeRulesVersion()
	latest := githubRelease("elastic", "vale-rules")
	return compare("Elastic Vale rules", installed, latest, "Run `elastic-docs-utils update --component vale-rules`")
}

func checkSkills() Item {
	current, err := state.Load()
	if err != nil {
		return Item{Name: "Elastic Docs skills", Installed: "managed", State: "unknown", Hint: "Run `elastic-docs-utils sync` to refresh skills"}
	}
	latest := githubCommit("elastic", "elastic-docs-skills")
	return skillStatus(current.Skills, latest)
}

func checkElasticDocsUtils(version string) Item {
	latest := githubRelease("elastic", "docs-utils")
	if version == "dev" {
		return Item{Name: "Elastic Docs Utils", Installed: "local build", Latest: latest, State: "local", Hint: "Builds from a checkout are not compared to releases"}
	}
	return compare("Elastic Docs Utils", version, latest, "Run the installer to update Elastic Docs Utils")
}

func skillStatus(records map[string]state.SkillState, latest string) Item {
	item := Item{Name: "Elastic Docs skills", Installed: "managed", Latest: shortRevision(latest), State: "unknown", Hint: "Run `elastic-docs-utils sync` to refresh skills"}
	if latest == "" {
		return item
	}
	var installed string
	for _, record := range records {
		if record.Source != "https://github.com/elastic/elastic-docs-skills.git" || record.Commit == "" {
			continue
		}
		installed = record.Commit
		if record.Commit != latest {
			item.Installed, item.State = shortRevision(record.Commit), "update available"
			return item
		}
	}
	if installed == "" {
		return item
	}
	item.Installed, item.State = shortRevision(installed), "current"
	return item
}

func shortRevision(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func compare(name, installed, latest, hint string) Item {
	item := Item{Name: name, Installed: installed, Latest: latest, Hint: hint}
	switch {
	case installed == "":
		item.Installed, item.State = "not installed", "missing"
	case latest == "":
		item.State = "unknown"
	case semverGT(latest, installed):
		item.State = "update available"
	default:
		item.State = "current"
	}
	return item
}

func binaryVersion(command string, args ...string) string {
	path, err := exec.LookPath(command)
	if err != nil {
		return ""
	}
	out, err := exec.Command(path, args...).Output()
	if err != nil {
		return ""
	}
	return parseVersion(string(out))
}

func githubRelease(owner, repo string) string {
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return ""
	}
	return parseVersion(payload.TagName)
}

func githubCommit(owner, repo string) string {
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/main", owner, repo))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var payload struct {
		SHA string `json:"sha"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return ""
	}
	return payload.SHA
}

func valeRulesVersion() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{
		os.Getenv("VALE_STYLES_PATH"),
		filepath.Join(home, "Library", "Application Support", "vale", "styles"),
		filepath.Join(home, ".local", "share", "vale", "styles"),
		filepath.Join(home, ".vale", "styles"),
	}
	for _, base := range candidates {
		if base == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(base, "Elastic", "VERSION"))
		if err == nil {
			return parseVersion(string(data))
		}
	}
	return ""
}

var versionRE = regexp.MustCompile(`\d+\.\d+\.\d+`)

func parseVersion(value string) string { return versionRE.FindString(value) }

func semverGT(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	var av, bv [3]int
	fmt.Sscanf(strings.TrimPrefix(a, "v"), "%d.%d.%d", &av[0], &av[1], &av[2])
	fmt.Sscanf(strings.TrimPrefix(b, "v"), "%d.%d.%d", &bv[0], &bv[1], &bv[2])
	for i := range av {
		if av[i] != bv[i] {
			return av[i] > bv[i]
		}
	}
	return false
}

func save(status Status) error {
	path, err := paths.UpdateStatusPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-*")
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
