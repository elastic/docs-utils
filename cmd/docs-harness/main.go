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

// docs-harness is the bootstrap installer for the Elastic Docs Harness.
// Run once to configure Claude Code for Elastic documentation workflows:
// MCP servers, skill catalogs, provider toggle, and update checks.
//
// Usage:
//
//	docs-harness [options]
//	docs-harness check-updates
//
// Options:
//
//	--provider claude|litellm   Provider to use (default: prompted)
//	--gateway-url URL           LiteLLM gateway base URL (required for --provider=litellm)
//	--no-telemetry              Disable OTel tagging
//	--update                    Update skill catalogs and check tool versions, then exit
//	--yes                       Non-interactive; accept all defaults
//	--dry-run                   Show what would be done without making changes
//	--force                     Overwrite existing MCP entries with different URLs
//	--version                   Print version and exit
//	--help                      Show this help
package main

import (
	"flag"
	"fmt"
	"os"
	"github.com/elastic/docs-harness/internal/config"
	"github.com/elastic/docs-harness/internal/hooks"
	"github.com/elastic/docs-harness/internal/marketplace"
	"github.com/elastic/docs-harness/internal/mcp"
	"github.com/elastic/docs-harness/internal/preflight"
	"github.com/elastic/docs-harness/internal/prompt"
	"github.com/elastic/docs-harness/internal/provider"
	"github.com/elastic/docs-harness/internal/telemetry"
	"github.com/elastic/docs-harness/internal/tools"
)

// Version is stamped by GoReleaser at build time via ldflags.
var Version = "dev"

func main() {
	if err := run(); err != nil {
		prompt.Err("%v", err)
		os.Exit(1)
	}
}

func run() error {
	// ── Preflight ────────────────────────────────────────────────────────────
	// Check prerequisites before parsing all flags so --help still works.

	// ── Flags ────────────────────────────────────────────────────────────────
	var (
		flagProvider    = flag.String("provider", "", "Provider: claude or litellm")
		flagGatewayURL  = flag.String("gateway-url", "", "LiteLLM gateway base URL")
		flagNoTelemetry = flag.Bool("no-telemetry", false, "Disable OTel tagging (not recommended)")
		flagYes         = flag.Bool("yes", false, "Non-interactive, accept all defaults")
		flagDryRun      = flag.Bool("dry-run", false, "Show what would happen without making changes")
		flagForce       = flag.Bool("force", false, "Overwrite existing MCP entries with different URLs")
		flagUpdate      = flag.Bool("update", false, "Update skill catalogs and check tool versions, then exit")
		flagVersion     = flag.Bool("version", false, "Print version and exit")
	)
	flag.Usage = usage
	flag.Parse()

	if *flagVersion {
		fmt.Println("docs-harness", Version)
		return nil
	}

	subcommand := flag.Arg(0)

	// Preflight: check Claude Code is installed (skip for check-updates subcommand
	// which is called from hooks where the user already has Claude Code).
	if subcommand != "check-updates" {
		if err := preflight.Check(*flagYes); err != nil {
			return err
		}
	}

	// ── Subcommand: check-updates ────────────────────────────────────────────
	if subcommand == "check-updates" {
		return runCheckUpdates()
	}

	// ── Flag: --update ───────────────────────────────────────────────────────
	if *flagUpdate {
		return runUpdate(*flagDryRun)
	}

	// ── Default: full install / reconfigure ─────────────────────────────────
	if *flagDryRun {
		prompt.Warn("DRY RUN — no changes will be written")
	}

	printBanner(Version)

	// 1. Determine provider.
	providerChoice := *flagProvider
	if providerChoice == "" {
		idx := prompt.Select(
			"Which provider should Claude Code use?",
			[]string{"In-house Claude (default)", "Elastic LiteLLM gateway"},
			0, *flagYes,
		)
		if idx == 1 {
			providerChoice = provider.ProviderLiteLLM
		} else {
			providerChoice = provider.ProviderClaude
		}
	}

	gatewayURL := *flagGatewayURL
	if providerChoice == provider.ProviderLiteLLM && gatewayURL == "" {
		gatewayURL = prompt.String(
			"LiteLLM gateway base URL",
			"",
			*flagYes,
		)
	}

	// 2. Catalogs: always public only (internal catalog has no plugin manifest).
	selectedCatalogs := []string{"public"}

	// 3. Telemetry: always on unless explicitly disabled via --no-telemetry.
	enableTelemetry := !*flagNoTelemetry

	// ── Apply ────────────────────────────────────────────────────────────────
	prompt.Info("Registering marketplaces and installing plugins...")
	if err := marketplace.EnsureAll(selectedCatalogs, *flagDryRun); err != nil {
		return fmt.Errorf("marketplace setup: %w", err)
	}

	prompt.Info("Configuring MCP servers in ~/.claude.json...")
	if err := mcp.EnsureGlobal(*flagForce, *flagDryRun); err != nil {
		return fmt.Errorf("MCP setup: %w", err)
	}

	prompt.Info("Applying provider config to settings.json...")
	if err := provider.Apply(providerChoice, gatewayURL, *flagDryRun); err != nil {
		return fmt.Errorf("provider setup: %w", err)
	}

	if enableTelemetry {
		prompt.Info("Applying telemetry tags...")
		if err := telemetry.Apply(Version, *flagDryRun); err != nil {
			return fmt.Errorf("telemetry setup: %w", err)
		}
	}

	prompt.Info("Extracting hook scripts...")
	if err := hooks.Extract(*flagDryRun); err != nil {
		return fmt.Errorf("hook extraction: %w", err)
	}
	if err := hooks.RegisterInSettings(*flagDryRun); err != nil {
		return fmt.Errorf("hook registration: %w", err)
	}

	prompt.Info("Checking tool versions...")
	statuses := tools.CheckAll()
	tools.PrintSummary(statuses)

	// Write harness config.
	cfg := config.HarnessConfig{
		Provider:       providerChoice,
		LiteLLMBaseURL: gatewayURL,
		Dispatch:       config.DispatchConfig{Enabled: true, Mode: "soft"},
		Catalogs:       selectedCatalogs,
		UpdateCheck:    true,
		Telemetry:      enableTelemetry,
		HarnessVersion: Version,
	}
	if err := config.Save(cfg, *flagDryRun); err != nil {
		return fmt.Errorf("saving harness config: %w", err)
	}
	prompt.OK("Harness config written to ~/.claude/docs-harness.json")

	fmt.Println()
	prompt.OK("Setup complete! Restart Claude Code for changes to take effect.")
	fmt.Println()
	fmt.Println("  /docs-setup    — reconfigure interactively from inside Claude")
	fmt.Println("  /docs-config   — toggle provider, dispatch, or catalogs")
	fmt.Println("  /docs-update   — check and apply updates")
	fmt.Println()

	return nil
}

// runUpdate updates skill catalogs and checks tool versions. Called by --update.
func runUpdate(dryRun bool) error {
	if dryRun {
		prompt.Warn("DRY RUN — no changes will be written")
	}

	prompt.Info("Updating skill catalogs...")
	if err := marketplace.Update(dryRun); err != nil {
		prompt.Warn("Skill catalog update failed: %v", err)
	}

	prompt.Info("Checking tool versions...")
	statuses := tools.CheckAll()
	tools.PrintSummary(statuses)

	fmt.Println()
	prompt.OK("Update complete.")
	return nil
}

// runCheckUpdates is called by the SessionStart hook to print a one-line notice
// when tools are behind. Silent when everything is current.
func runCheckUpdates() error {
	cfg, _ := config.Load(Version)
	if !cfg.UpdateCheck {
		return nil
	}
	statuses := tools.CheckAll()
	tools.PrintSummary(statuses)
	return nil
}

func printBanner(version string) {
	cyan := "\033[0;36m"
	bold := "\033[1m"
	reset := "\033[0m"
	fmt.Printf(`
%s  ╔══════════════════════════════════════════════════╗
  ║                                                  ║
  ║   ⚡  Elastic AI Docs Setup                      ║
  ║       Configure Claude Code for documentation    ║
  ║                                                  ║
  ╚══════════════════════════════════════════════════╝%s
  %sv%s%s

`, cyan, reset, bold, version, reset)
}

func usage() {
	fmt.Fprintf(os.Stderr, `Elastic Docs Harness v%s

Bootstrap Claude Code for Elastic documentation workflows.

Usage:
  docs-harness [options]         Full install / reconfigure
  docs-harness --update          Update skill catalogs and check tool versions
  docs-harness check-updates     Version check only (used by SessionStart hook)

Options:
  --update                       Update skill catalogs and check tool versions, then exit
  --provider claude|litellm      Provider to use (default: prompted)
  --gateway-url URL              LiteLLM gateway base URL
  --no-telemetry                 Disable OTel resource-attribute tagging (not recommended)
  --yes                          Non-interactive, accept all defaults
  --dry-run                      Show what would be done without making changes
  --force                        Overwrite existing MCP entries with different URLs
  --version                      Print version and exit
  --help                         Show this help

`, Version)
}
