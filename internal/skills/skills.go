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

// Package skills installs Elastic Docs skills into the shared Agent Skills
// location and links host-specific discovery directories.
package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/elastic/docs-utils/internal/hosts"
	"github.com/elastic/docs-utils/internal/paths"
	"github.com/elastic/docs-utils/internal/state"
)

const PublicRepo = "https://github.com/elastic/elastic-docs-skills.git"
const InternalRepo = "https://github.com/elastic/elastic-docs-skills-internal.git"
const markerFile = ".elastic-docs-utils.json"

type marker struct {
	Product string `json:"product"`
}

type Result struct {
	Installed []string
	Linked    []string
	Records   map[string]state.SkillState
	Files     []string
}

// ProgressFunc reports the current item within a synchronization phase.
// Current is one-based; totals can change when a catalog has been fetched and
// its skill count becomes known.
type ProgressFunc func(current, total int, label string)

// Sync fetches enabled catalogs, installs all skills into ~/.agents/skills,
// then exposes them to hosts that do not discover that location directly.
func Sync(targets []hosts.ID, internal, dryRun bool) (Result, error) {
	return SyncWithProgress(targets, internal, dryRun, nil)
}

// SyncWithProgress performs the same synchronization as Sync while reporting
// catalog fetches, individual skill installations, and host discovery links.
func SyncWithProgress(targets []hosts.ID, internal, dryRun bool, progress ProgressFunc) (Result, error) {
	result := Result{Records: map[string]state.SkillState{}}
	repos := repositories(internal)
	for repoIndex, repo := range repos {
		catalog := catalogLabel(repo)
		reportProgress(progress, repoIndex+1, len(repos), "Fetching "+catalog+" skill catalog")
		staging, err := clone(repo, dryRun)
		if err != nil {
			return result, err
		}
		if dryRun {
			continue
		}
		commit, err := gitRevision(staging)
		if err != nil {
			_ = os.RemoveAll(staging)
			return result, err
		}
		entries, err := findSkills(staging)
		if err != nil {
			_ = os.RemoveAll(staging)
			return result, err
		}
		root, err := paths.CanonicalSkillsDir()
		if err != nil {
			return result, err
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return result, err
		}
		names := make([]string, 0, len(entries))
		for name := range entries {
			names = append(names, name)
		}
		sort.Strings(names)
		for skillIndex, name := range names {
			reportProgress(progress, skillIndex+1, len(names), "Installing "+catalog+" skill: "+name)
			source := entries[name]
			destination := filepath.Join(root, name)
			if err := replaceDir(source, destination); err != nil {
				return result, err
			}
			result.Installed = append(result.Installed, name)
			result.Records[name] = state.SkillState{Source: repo, Commit: commit}
			result.Files = append(result.Files, destination)
		}
		if err := os.RemoveAll(staging); err != nil {
			return result, err
		}
	}
	if dryRun {
		return result, nil
	}
	root, err := paths.CanonicalSkillsDir()
	if err != nil {
		return result, err
	}
	for targetIndex, id := range targets {
		reportProgress(progress, targetIndex+1, len(targets), "Linking skills for "+string(id))
		for _, target := range linkRoots(id) {
			links, err := linkAll(root, target)
			if err != nil {
				return result, err
			}
			result.Linked = append(result.Linked, string(id))
			result.Files = append(result.Files, links...)
		}
	}
	return result, nil
}

func reportProgress(progress ProgressFunc, current, total int, label string) {
	if progress != nil {
		progress(current, total, label)
	}
}

func catalogLabel(repo string) string {
	if repo == InternalRepo {
		return "internal"
	}
	return "public"
}

func gitRevision(directory string) (string, error) {
	out, err := exec.Command("git", "-C", directory, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read skill catalog revision: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func repositories(internal bool) []string {
	return ActiveRepos(internal)
}

// ActiveRepos returns the catalog URLs that a sync run covers.
// Exported so callers can compute the same set when pruning.
func ActiveRepos(internal bool) []string {
	repos := []string{PublicRepo}
	if internal {
		repos = append(repos, InternalRepo)
	}
	return repos
}

func clone(repo string, dryRun bool) (string, error) {
	if dryRun {
		return "", nil
	}
	cache, err := paths.CacheDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(cache, "skills-*")
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", "clone", "--depth", "1", repo, staging)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("fetch skills from %s: %w: %s", repo, err, strings.TrimSpace(string(out)))
	}
	return staging, nil
}

func findSkills(root string) (map[string]string, error) {
	entries := map[string]string{}
	err := filepath.WalkDir(filepath.Join(root, "skills"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		name := filepath.Base(filepath.Dir(path))
		if _, exists := entries[name]; exists {
			return fmt.Errorf("duplicate skill %q", name)
		}
		entries[name] = filepath.Dir(path)
		return nil
	})
	return entries, err
}

func replaceDir(source, destination string) error {
	if err := ensureOwnedOrMissing(destination); err != nil {
		return err
	}
	staging := destination + ".new"
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := copyDir(source, staging); err != nil {
		return err
	}
	data, err := json.Marshal(marker{Product: paths.Product})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, markerFile), data, 0o644); err != nil {
		return err
	}
	backup := destination + ".previous"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, destination); err != nil {
		return err
	}
	return os.RemoveAll(backup)
}

func ensureOwnedOrMissing(destination string) error {
	_, err := os.Stat(destination)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(destination, markerFile))
	if err != nil {
		return fmt.Errorf("refusing to replace unmanaged skill %q", filepath.Base(destination))
	}
	var owned marker
	if err := json.Unmarshal(data, &owned); err != nil || owned.Product != paths.Product {
		return fmt.Errorf("refusing to replace unmanaged skill %q", filepath.Base(destination))
	}
	return nil
}

func copyDir(source, destination string) error {
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func linkRoots(id hosts.ID) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	switch id {
	case hosts.Claude:
		return []string{filepath.Join(home, ".claude", "skills")}
	case hosts.Cursor:
		return []string{filepath.Join(home, ".cursor", "skills")}
	default:
		return nil
	}
}

func linkAll(source, target string) ([]string, error) {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return nil, err
	}
	links := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		link := filepath.Join(target, entry.Name())
		if _, err := os.Lstat(link); err == nil {
			continue
		}
		if err := os.Symlink(filepath.Join(source, entry.Name()), link); err != nil {
			if err := copyDir(filepath.Join(source, entry.Name()), link); err != nil {
				return nil, err
			}
		}
		links = append(links, link)
	}
	return links, nil
}

// Stale returns the names of skills that were installed from one of activeRepos
// but are no longer present in current (the result of the most recent Sync).
// Skills from repos not in activeRepos are untouched — e.g. internal skills are
// left alone when syncing without --internal.
func Stale(current map[string]state.SkillState, existing map[string]state.SkillState, activeRepos []string) []string {
	active := make(map[string]bool, len(activeRepos))
	for _, r := range activeRepos {
		active[r] = true
	}
	var names []string
	for name, record := range existing {
		if !active[record.Source] {
			continue
		}
		if _, stillPresent := current[name]; !stillPresent {
			names = append(names, name)
		}
	}
	return names
}

// RemoveOwned deletes only skill directories carrying this product's marker.
func RemoveOwned(names []string, dryRun bool) error {
	root, err := paths.CanonicalSkillsDir()
	if err != nil {
		return err
	}
	for _, name := range names {
		destination := filepath.Join(root, name)
		if err := ensureOwnedOrMissing(destination); err != nil {
			return err
		}
		if dryRun {
			continue
		}
		for _, host := range []hosts.ID{hosts.Claude, hosts.Cursor} {
			for _, linkRoot := range linkRoots(host) {
				link := filepath.Join(linkRoot, name)
				if info, err := os.Lstat(link); err == nil && info.Mode()&os.ModeSymlink != 0 {
					if err := os.Remove(link); err != nil {
						return err
					}
				}
			}
		}
		if err := os.RemoveAll(destination); err != nil {
			return err
		}
	}
	return nil
}
