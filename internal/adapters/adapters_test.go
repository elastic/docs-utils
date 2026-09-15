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

package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/elastic/docs-utils/internal/hosts"
)

func TestMergeServerRequiresForceForConflict(t *testing.T) {
	servers := map[string]any{"elastic-docs": map[string]any{"url": "https://old.example"}}
	wanted := map[string]any{"url": "https://new.example"}
	if err := mergeServer(servers, "elastic-docs", wanted, false); err == nil {
		t.Fatal("conflicting server was accepted without force")
	}
	if err := mergeServer(servers, "elastic-docs", wanted, true); err != nil {
		t.Fatal(err)
	}
	if got := servers["elastic-docs"].(map[string]any)["url"]; got != "https://new.example" {
		t.Fatalf("URL = %q, want replacement", got)
	}
}

func TestMergeServerPreservesValidLegacyElasticDocsEndpoint(t *testing.T) {
	servers := map[string]any{"elastic-docs": map[string]any{"url": legacyPublicMCP, "headers": map[string]any{}}}
	wanted := map[string]any{"url": publicMCP}
	if err := mergeServer(servers, "elastic-docs", wanted, false); err != nil {
		t.Fatal(err)
	}
	if got := servers["elastic-docs"].(map[string]any)["url"]; got != legacyPublicMCP {
		t.Fatalf("URL = %q, want existing valid endpoint retained", got)
	}
}

func TestOpenCodeConfigPath(t *testing.T) {
	path, err := openCodeConfigPath("/example/home")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		want := filepath.Join("/example/home", ".config", "opencode", "opencode.json")
		if path != want {
			t.Fatalf("path = %q, want %q", path, want)
		}
	}
}

func TestValidateListOutput(t *testing.T) {
	if err := validateListOutput("elastic-docs connected", false); err != nil {
		t.Fatal(err)
	}
	if err := validateListOutput("elastic-docs connected", true); err == nil {
		t.Fatal("missing internal server was accepted")
	}
}

func TestSyncReportsHostProgress(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var got []string
	_, err := SyncWithProgress([]hosts.ID{hosts.Cursor, hosts.OpenCode}, false, true, false, "/tmp/elastic-docs-utils", func(current, total int, label string) {
		got = append(got, fmt.Sprintf("%d/%d %s", current, total, label))
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1/2 Configuring cursor", "2/2 Configuring opencode"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("progress = %v, want %v", got, want)
	}
}

func TestWriteClaudeHookUsesAndUpgradesAbsoluteCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := "elastic-docs-utils --color=never hook session-start --host claude"
	if err := os.WriteFile(path, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"`+legacy+`"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	command := "'/Applications/Elastic Docs Utils/elastic-docs-utils' --color=never hook session-start --host claude"
	if err := writeClaudeHook(path, command); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), command) || strings.Contains(string(data), `"command": "elastic-docs-utils`) {
		t.Fatalf("hook was not upgraded: %s", data)
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/Users/Ada's Tools/elastic-docs-utils"); got != "'/Users/Ada'\\''s Tools/elastic-docs-utils'" {
		t.Fatalf("quoted command = %q", got)
	}
}

func TestClaudeHookCommandRequiresExecutable(t *testing.T) {
	if _, err := claudeHookCommand(""); err == nil {
		t.Fatal("empty executable was accepted")
	}
}

func TestParseInstalledPlugins(t *testing.T) {
	output := `[{"id":"elastic-docs-skills@elastic-docs-skills","version":"1.0.11","scope":"user","enabled":true},{"id":"other-plugin@some-marketplace","version":"1.0.0","scope":"user","enabled":false}]`
	found, err := parseInstalledPlugins(output, "elastic-docs-skills")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected elastic-docs-skills to be found")
	}
	found, err = parseInstalledPlugins(output, "missing-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected missing-plugin to not be found")
	}
	if _, err := parseInstalledPlugins("not-json", "any"); err == nil {
		t.Fatal("invalid JSON was accepted")
	}
}
