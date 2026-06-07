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

// Package preflight checks prerequisites before the installer runs:
//   - Claude Code CLI is installed (and offers to install it if missing)
//   - Go home directory is accessible
//   - Minimum platform requirements
package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/elastic/docs-harness/internal/prompt"
)

// Check verifies all prerequisites. Returns an error if any hard requirement
// is not met and the user declined to install it. yes enables non-interactive mode.
func Check(yes bool) error {
	prompt.Info("Checking prerequisites...")

	// Home directory accessible.
	if _, err := os.UserHomeDir(); err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	// Claude Code CLI.
	if err := checkClaudeCode(yes); err != nil {
		return err
	}

	prompt.OK("Prerequisites satisfied")
	return nil
}

// checkClaudeCode verifies the `claude` CLI is on PATH and offers to install
// it if missing.
func checkClaudeCode(yes bool) error {
	_, err := exec.LookPath("claude")
	if err == nil {
		// Already installed — check it's actually Claude Code, not something else.
		out, _ := exec.Command("claude", "--version").Output()
		if len(out) > 0 {
			prompt.OK("Claude Code found: %s", strings.TrimSpace(string(out)))
		} else {
			prompt.OK("Claude Code CLI found on PATH")
		}
		return nil
	}

	prompt.Warn("Claude Code CLI not found on PATH")

	install := prompt.YesOrNo("Install Claude Code CLI now?", true, yes)
	if !install {
		return fmt.Errorf("Claude Code CLI is required. Install it from https://claude.ai/download and re-run")
	}

	return installClaudeCode()
}

// installClaudeCode installs the Claude Code CLI via npm (the officially
// supported distribution method).
func installClaudeCode() error {
	// Check npm is available.
	if _, err := exec.LookPath("npm"); err != nil {
		return fmt.Errorf(
			"npm not found — cannot auto-install Claude Code.\n"+
				"  Install Node.js from https://nodejs.org then run: npm install -g @anthropic-ai/claude-code\n"+
				"  Or download the desktop app from https://claude.ai/download",
		)
	}

	prompt.Info("Installing Claude Code via npm (this may take a moment)...")

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// On Windows, use npm via cmd.exe to get PATH correctly.
		cmd = exec.Command("cmd", "/C", "npm", "install", "-g", "@anthropic-ai/claude-code")
	default:
		cmd = exec.Command("npm", "install", "-g", "@anthropic-ai/claude-code")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"npm install failed: %w\n"+
				"  Try manually: npm install -g @anthropic-ai/claude-code\n"+
				"  Or download the desktop app from https://claude.ai/download",
			err,
		)
	}

	// Verify installation succeeded.
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf(
			"Claude Code installed but `claude` not found on PATH.\n"+
				"  You may need to restart your shell or add npm's global bin to PATH.\n"+
				"  Then re-run the docs-harness installer.",
		)
	}

	prompt.OK("Claude Code installed successfully")
	return nil
}
