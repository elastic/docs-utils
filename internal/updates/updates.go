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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/elastic/docs-utils/internal/paths"
	"github.com/elastic/docs-utils/internal/state"
)

const CacheTTL = 24 * time.Hour

// versionProbeTimeout bounds a `--version` call. A tool that starts a server
// instead of reporting its version must not hang the whole command.
const versionProbeTimeout = 10 * time.Second

// errRateLimited marks the one lookup failure a user can act on directly.
var errRateLimited = errors.New("GitHub API rate limit reached")

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

// ProgressFunc reports the item currently being checked. Current is one-based.
type ProgressFunc func(current, total int, name string)

type updateCheck struct {
	name string
	run  func() *Item
}

// Refresh performs bounded network checks and atomically persists the result.
func Refresh(version string) (Status, error) {
	return RefreshWithProgress(version, nil)
}

// RefreshWithProgress performs the same checks as Refresh and reports each
// component before its local and network probes begin.
func RefreshWithProgress(version string, progress ProgressFunc) (Status, error) {
	current, _ := state.Load()
	checks := []updateCheck{
		{"Elastic Docs Utils", func() *Item { item := checkElasticDocsUtils(version); return &item }},
		{"docs-builder", func() *Item { item := checkDocsBuilder(); return &item }},
		{"Vale", func() *Item { item := checkVale(); return &item }},
		{"Elastic Vale rules", func() *Item { item := checkValeRules(); return &item }},
		{"Elastic Docs skills", func() *Item { item := checkSkills(current); return &item }},
	}
	if prefs, err := state.LoadPreferences(); err == nil && prefs.Internal {
		checks = append(checks, updateCheck{"Elastic Docs internal skills", func() *Item { return checkInternalSkills(current) }})
	}
	items := runChecks(checks, progress)
	status := Status{CheckedAt: time.Now().UTC(), Items: items}
	if err := save(status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func runChecks(checks []updateCheck, progress ProgressFunc) []Item {
	items := make([]Item, 0, len(checks))
	for index, check := range checks {
		if progress != nil {
			progress(index+1, len(checks), check.name)
		}
		if item := check.run(); item != nil {
			items = append(items, *item)
		}
	}
	return items
}

func checkDocsBuilder() Item {
	installed := binaryVersion("docs-builder", "--version")
	latest, err := githubRelease("elastic", "docs-builder")
	return compare("docs-builder", installed, latest, err, hints{
		missing: "Not installed. Run `elastic-docs-utils install --with-docs-builder`",
		update:  "Run `elastic-docs-utils update --component docs-builder`",
	})
}

func checkVale() Item {
	installed := binaryVersion("vale", "--version")
	latest, err := githubRelease("errata-ai", "vale")
	return compare("Vale", installed, latest, err, hints{
		missing: "Not installed. Run `elastic-docs-utils install --with-vale`",
		update:  "Update Vale with your package manager",
	})
}

func checkValeRules() Item {
	installed := valeRulesVersion()
	latest, err := githubRelease("elastic", "vale-rules")
	return compare("Elastic Vale rules", installed, latest, err, hints{
		missing: "Not installed. Run `elastic-docs-utils install --with-vale`",
		update:  "Run `elastic-docs-utils update --component vale-rules`",
	})
}

const publicSkillsSource = "https://github.com/elastic/elastic-docs-skills.git"
const internalSkillsSource = "https://github.com/elastic/elastic-docs-skills-internal.git"

func checkSkills(current state.State) Item {
	latest, lookupErr := githubCommit("elastic", "elastic-docs-skills")
	if lookupErr != nil {
		return Item{Name: "Elastic Docs skills", Installed: "managed", State: "unknown", Hint: lookupHint(lookupErr)}
	}
	return repoSkillStatus("Elastic Docs skills", publicSkillsSource, current.Skills, latest)
}

// checkInternalSkills returns an update Item for the internal skills repo when
// the user has enabled internal access AND a gh CLI token is available. It
// returns nil — without logging — when either condition is absent, so users
// without Elastic org access see no noise.
func checkInternalSkills(current state.State) *Item {
	prefs, err := state.LoadPreferences()
	if err != nil || !prefs.Internal {
		return nil
	}
	if githubToken() == "" {
		return nil
	}
	latest, err := githubCommit("elastic", "elastic-docs-skills-internal")
	if err != nil {
		// 404 / 403 means no repo access — a normal state, not an error to surface.
		return nil
	}
	item := repoSkillStatus("Elastic Docs internal skills", internalSkillsSource, current.Skills, latest)
	return &item
}

func checkElasticDocsUtils(version string) Item {
	if version == "dev" {
		latest, _ := githubRelease("elastic", "docs-utils")
		return Item{Name: "Elastic Docs Utils", Installed: "local build", Latest: latest, State: "local", Hint: "Builds from a checkout are not compared to releases"}
	}
	latest, err := githubRelease("elastic", "docs-utils")
	return compare("Elastic Docs Utils", version, latest, err, hints{
		missing: "Run the installer to install Elastic Docs Utils",
		update:  "Run `elastic-docs-utils update --component elastic-docs-utils`",
	})
}

func repoSkillStatus(name, source string, records map[string]state.SkillState, latest string) Item {
	item := Item{Name: name, Installed: "managed", Latest: shortRevision(latest), State: "unknown", Hint: "Run `elastic-docs-utils sync` to refresh skills"}
	if latest == "" {
		return item
	}
	var installed string
	for _, record := range records {
		if record.Source != source || record.Commit == "" {
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

// hints carry the next step for each actionable state. A component the user
// never asked for reports the install command rather than the update command,
// so "not installed" does not read as a failed install.
type hints struct {
	missing string
	update  string
}

func compare(name, installed, latest string, latestErr error, h hints) Item {
	item := Item{Name: name, Installed: installed, Latest: latest}
	switch {
	case installed == "":
		item.Installed, item.State, item.Hint = "not installed", "missing", h.missing
	case latestErr != nil:
		item.State, item.Hint = "unknown", lookupHint(latestErr)
	case latest == "":
		item.State, item.Hint = "unknown", "Could not determine the latest version from GitHub"
	case semverGT(latest, installed):
		item.State, item.Hint = "update available", h.update
	default:
		item.State = "current"
	}
	return item
}

// lookupHint separates "we could not ask GitHub" from "the tool is missing",
// which otherwise both surface as an unexplained unknown.
func lookupHint(err error) string {
	if errors.Is(err, errRateLimited) {
		return "GitHub API rate limit reached; set GITHUB_TOKEN or retry later"
	}
	return "Could not query GitHub to determine the latest version"
}

// UpdateAvailable reports whether a release newer than installed exists for the
// given GitHub project, using the same comparison as the status table. A tool
// that is absent, or a lookup this cannot complete, reports true so that callers
// run the installer rather than silently skip a real update.
func UpdateAvailable(owner, repo, installed string) bool {
	if installed == "" {
		return true
	}
	latest, err := githubRelease(owner, repo)
	if err != nil || latest == "" {
		return true
	}
	return semverGT(latest, installed)
}

// BinaryVersion reports the version a locally installed tool prints, or an
// empty string when the tool is absent or prints nothing usable. Callers use it
// to confirm that an installer actually changed the binary, because the
// upstream installers exit 0 whether they install, skip, or are declined.
func BinaryVersion(command string, args ...string) string {
	return binaryVersion(command, args...)
}

func binaryVersion(command string, args ...string) string {
	path, err := exec.LookPath(command)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionProbeTimeout)
	defer cancel()
	// Some tools report their version on stderr, and others interleave startup
	// logging with it, so parse the combined stream and tolerate a non-zero
	// exit as long as the tool printed something.
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if err != nil {
		if len(out) == 0 || ctx.Err() != nil {
			return ""
		}
	}
	return parseVersion(string(out))
}

func githubRelease(owner, repo string) (string, error) {
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := githubJSON(fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo), &payload); err != nil {
		return "", err
	}
	return parseVersion(payload.TagName), nil
}

func githubCommit(owner, repo string) (string, error) {
	var payload struct {
		SHA string `json:"sha"`
	}
	if err := githubJSON(fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/main", owner, repo), &payload); err != nil {
		return "", err
	}
	return payload.SHA, nil
}

func githubJSON(url string, payload any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// A token is optional, but the unauthenticated limit is 60 requests per
	// hour for the whole machine, which a few checks a day can exhaust.
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if rateLimited(resp) {
			return errRateLimited
		}
		return fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(payload)
}

func githubToken() string {
	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ghCLIToken()
}

// ghCLITokenOnce caches the result of `gh auth token` for the process lifetime
// to avoid repeated subprocess calls during a single update check.
var ghCLITokenOnce struct {
	sync.Once
	value string
}

// ghCLIToken returns the token stored by the gh CLI, or an empty string if gh
// is absent, unauthenticated, or fails for any reason.
func ghCLIToken() string {
	ghCLITokenOnce.Do(func() {
		path, err := exec.LookPath("gh")
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), versionProbeTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, path, "auth", "token").Output()
		if err != nil {
			return
		}
		ghCLITokenOnce.value = strings.TrimSpace(string(out))
	})
	return ghCLITokenOnce.value
}

func rateLimited(resp *http.Response) bool {
	// 429 from the secondary rate limit may not carry X-RateLimit-Remaining.
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	return resp.Header.Get("X-RateLimit-Remaining") == "0"
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

var (
	versionRE = regexp.MustCompile(`\d+\.\d+\.\d+`)
	// A line holding only a version, optionally with build metadata, is the
	// reliable signal when a tool logs before reporting its version.
	versionLineRE = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)([+-].*)?$`)
)

func parseVersion(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if match := versionLineRE.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			return match[1]
		}
	}
	return versionRE.FindString(value)
}

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
