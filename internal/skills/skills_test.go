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
