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

// Package state persists Elastic Docs Utils ownership and preferences.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/elastic/docs-utils/internal/paths"
)

const SchemaVersion = 2

// Preferences are user-selected capabilities. Host-specific data lives in
// State so sync can safely distinguish user choices from owned artifacts.
type Preferences struct {
	SchemaVersion int      `json:"schemaVersion"`
	Hosts         []string `json:"hosts"`
	Internal      bool     `json:"internal"`
}

// State records files and links created by this product.
type State struct {
	SchemaVersion  int                   `json:"schemaVersion"`
	MigratedLegacy bool                  `json:"migratedLegacy"`
	Hosts          map[string]HostState  `json:"hosts"`
	Skills         map[string]SkillState `json:"skills"`
	UpdatedAt      time.Time             `json:"updatedAt"`
}

type HostState struct {
	Files []string `json:"files"`
}

type SkillState struct {
	Source  string   `json:"source"`
	Commit  string   `json:"commit,omitempty"`
	Version string   `json:"version,omitempty"`
	Links   []string `json:"links"`
}

func DefaultPreferences() Preferences { return Preferences{SchemaVersion: SchemaVersion} }
func DefaultState() State {
	return State{SchemaVersion: SchemaVersion, Hosts: map[string]HostState{}, Skills: map[string]SkillState{}}
}

func LoadPreferences() (Preferences, error) {
	path, err := paths.PreferencesPath()
	if err != nil {
		return Preferences{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultPreferences(), nil
	}
	if err != nil {
		return Preferences{}, err
	}
	var prefs Preferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return Preferences{}, fmt.Errorf("parse preferences: %w", err)
	}
	return prefs, nil
}

func Load() (State, error) {
	path, err := paths.StatePath()
	if err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultState(), nil
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("parse state: %w", err)
	}
	if s.Hosts == nil {
		s.Hosts = map[string]HostState{}
	}
	if s.Skills == nil {
		s.Skills = map[string]SkillState{}
	}
	return s, nil
}

func SavePreferences(prefs Preferences) error {
	prefs.SchemaVersion = SchemaVersion
	sort.Strings(prefs.Hosts)
	path, err := paths.PreferencesPath()
	if err != nil {
		return err
	}
	return writeJSON(path, prefs)
}

func Save(s State) error {
	s.SchemaVersion = SchemaVersion
	s.UpdatedAt = time.Now().UTC()
	path, err := paths.StatePath()
	if err != nil {
		return err
	}
	return writeJSON(path, s)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
