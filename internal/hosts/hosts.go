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

// Package hosts detects and configures supported agent harnesses.
package hosts

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// ID identifies a supported agent harness.
type ID string

const (
	Claude   ID = "claude"
	Codex    ID = "codex"
	Cursor   ID = "cursor"
	OpenCode ID = "opencode"
)

type Definition struct {
	ID      ID
	Label   string
	Command string
}

var definitions = []Definition{
	{Claude, "Claude Code", "claude"},
	{Codex, "Codex", "codex"},
	{Cursor, "Cursor CLI", "agent"},
	{OpenCode, "OpenCode", "opencode"},
}

func All() []Definition { return append([]Definition(nil), definitions...) }

func Detect() []Definition {
	var found []Definition
	for _, host := range definitions {
		if _, err := exec.LookPath(host.Command); err == nil {
			found = append(found, host)
		}
	}
	return found
}

func Parse(value string) ([]ID, error) {
	if value == "" {
		return nil, nil
	}
	known := make(map[ID]bool)
	for _, d := range definitions {
		known[d.ID] = true
	}
	seen := map[ID]bool{}
	for _, part := range strings.Split(value, ",") {
		id := ID(strings.TrimSpace(part))
		if !known[id] {
			return nil, fmt.Errorf("unknown host %q; valid hosts: claude, codex, cursor, opencode", id)
		}
		seen[id] = true
	}
	ids := make([]ID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func Strings(ids []ID) []string {
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = string(id)
	}
	return values
}
