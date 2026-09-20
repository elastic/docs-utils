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

package skills

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/elastic/docs-utils/internal/paths"
	"github.com/elastic/docs-utils/internal/state"
)

func TestStale(t *testing.T) {
	existing := map[string]state.SkillState{
		"write-docs":   {Source: PublicRepo, Commit: "abc"},
		"old-skill":    {Source: PublicRepo, Commit: "abc"},
		"internal-one": {Source: InternalRepo, Commit: "abc"},
		"user-skill":   {Source: "https://github.com/user/my-skills.git", Commit: "abc"},
	}
	current := map[string]state.SkillState{
		"write-docs":   {Source: PublicRepo, Commit: "def"},
		"internal-one": {Source: InternalRepo, Commit: "def"},
	}

	// Public-only sync: old-skill is stale, internal-one is untouched.
	stale := Stale(current, existing, ActiveRepos(false))
	sort.Strings(stale)
	if len(stale) != 1 || stale[0] != "old-skill" {
		t.Fatalf("public-only stale = %v, want [old-skill]", stale)
	}

	// Internal sync: old-skill and internal-one's removal would both be caught
	// if current dropped internal-one — here current still has it, so only old-skill.
	stale = Stale(current, existing, ActiveRepos(true))
	sort.Strings(stale)
	if len(stale) != 1 || stale[0] != "old-skill" {
		t.Fatalf("internal stale = %v, want [old-skill]", stale)
	}

	// user-skill is never flagged regardless of active repos.
	for _, s := range stale {
		if s == "user-skill" {
			t.Fatal("user-skill should not be pruned")
		}
	}
}

func TestReplaceDirRefusesUnmanagedSkill(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := replaceDir(source, destination); err == nil {
		t.Fatal("replaceDir accepted an unmanaged directory")
	}
}

func TestReplaceDirUpdatesOwnedSkill(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, markerFile), []byte(`{"product":"elastic/docs-utils"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := replaceDir(source, destination); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new" {
		t.Fatalf("skill contents = %q, want new", contents)
	}
}

// Pruning runs automatically, so a directory this tool did not install must
// survive it — and must not abort the prune of everything else.
func TestRemoveOwnedSkipsUnmanagedSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := paths.CanonicalSkillsDir()
	if err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(root, "owned-skill")
	unmanaged := filepath.Join(root, "hand-written")
	for _, dir := range []string{owned, unmanaged} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(owned, markerFile), []byte(`{"product":"`+paths.Product+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	skipped, err := RemoveOwned([]string{"owned-skill", "hand-written"}, false)
	if err != nil {
		t.Fatalf("RemoveOwned returned an error for an unmanaged directory: %v", err)
	}
	if len(skipped) != 1 || skipped[0] != "hand-written" {
		t.Fatalf("skipped = %v, want [hand-written]", skipped)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatal("owned skill was not removed")
	}
	if _, err := os.Stat(unmanaged); err != nil {
		t.Fatal("unmanaged skill was removed")
	}
}

// A dry run reports what it would skip without deleting anything.
func TestRemoveOwnedDryRunDeletesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := paths.CanonicalSkillsDir()
	if err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(root, "owned-skill")
	if err := os.MkdirAll(owned, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(owned, markerFile), []byte(`{"product":"`+paths.Product+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveOwned([]string{"owned-skill"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(owned); err != nil {
		t.Fatal("dry run removed the skill")
	}
}

// A name with no directory on disk is not an error and is not reported as
// skipped: the state record is stale and the prune should clear it.
func TestRemoveOwnedTreatsMissingAsRemovable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skipped, err := RemoveOwned([]string{"never-installed"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped = %v, want empty", skipped)
	}
}
