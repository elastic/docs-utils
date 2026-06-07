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

// Package provider manages the LiteLLM ↔ in-house Claude provider toggle.
//
// Only ANTHROPIC_BASE_URL is written to settings.json (non-secret).
// The API token must be exported as ANTHROPIC_AUTH_TOKEN in the shell, sourced
// from the existing ELASTIC_LITELLM_API_KEY environment variable.
// The installer adds a guarded snippet to ~/.zshrc / ~/.bash_profile that does
// this automatically when ELASTIC_LITELLM_API_KEY is present.
package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elastic/docs-harness/internal/config"
	"github.com/elastic/docs-harness/internal/prompt"
)

const (
	ProviderClaude  = "claude"
	ProviderLiteLLM = "litellm"
)

// Apply writes (or removes) the ANTHROPIC_BASE_URL env entry in settings.json
// based on the chosen provider. For litellm, also writes the guarded rc snippet.
func Apply(provider, baseURL string, dryRun bool) error {
	claudeDir, err := config.ClaudeDir()
	if err != nil {
		return err
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	switch provider {
	case ProviderClaude:
		if err := config.RemoveEnvKeys(settingsPath, []string{"ANTHROPIC_BASE_URL"}, dryRun); err != nil {
			return err
		}
		prompt.OK("Provider set to in-house Claude (ANTHROPIC_BASE_URL removed)")

	case ProviderLiteLLM:
		if baseURL == "" {
			return fmt.Errorf("--gateway-url is required when --provider=litellm")
		}
		updates := map[string]any{
			"env": map[string]any{
				"ANTHROPIC_BASE_URL": baseURL,
			},
		}
		if err := config.MergeJSON(settingsPath, updates, dryRun); err != nil {
			return fmt.Errorf("updating settings.json: %w", err)
		}
		prompt.OK("Provider set to LiteLLM gateway: %s", baseURL)
		prompt.Info("Token: ANTHROPIC_AUTH_TOKEN will be set from ELASTIC_LITELLM_API_KEY via shell rc snippet")
		if err := ensureRCSnippet(dryRun); err != nil {
			prompt.Warn("Could not write rc snippet: %v", err)
		}

	default:
		return fmt.Errorf("unknown provider %q; valid values: claude, litellm", provider)
	}
	return nil
}

// ensureRCSnippet adds a guarded snippet to the user's shell rc file that
// exports ANTHROPIC_AUTH_TOKEN from ELASTIC_LITELLM_API_KEY.
// The snippet is idempotent (guarded by a marker comment).
func ensureRCSnippet(dryRun bool) error {
	marker := "# docs-harness: LiteLLM token bridge"
	snippet := "\n" + marker + `
if [ -n "$ELASTIC_LITELLM_API_KEY" ]; then
  export ANTHROPIC_AUTH_TOKEN="$ELASTIC_LITELLM_API_KEY"
fi
# end docs-harness
`
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".bash_profile"),
	}
	for _, rc := range candidates {
		if _, err := os.Stat(rc); os.IsNotExist(err) {
			continue
		}
		data, err := os.ReadFile(rc)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), marker) {
			prompt.OK("RC snippet already present: %s", rc)
			continue
		}
		if dryRun {
			prompt.Info("[dry-run] would append LiteLLM token snippet to %s", rc)
			continue
		}
		f, err := os.OpenFile(rc, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, werr := f.WriteString(snippet)
		f.Close()
		if werr != nil {
			return werr
		}
		prompt.OK("Added LiteLLM token snippet to %s", rc)
	}
	return nil
}

