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

// elastic-docs-utils configures Elastic documentation workflows across coding
// agent harnesses.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/elastic/docs-utils/internal/adapters"
	"github.com/elastic/docs-utils/internal/bootstrap"
	"github.com/elastic/docs-utils/internal/hosts"
	"github.com/elastic/docs-utils/internal/paths"
	"github.com/elastic/docs-utils/internal/skills"
	"github.com/elastic/docs-utils/internal/state"
	"github.com/elastic/docs-utils/internal/ui"
	"github.com/elastic/docs-utils/internal/updates"
)

// Version is set by GoReleaser.
var Version = "dev"

type globalFlags struct {
	color   string
	verbose bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		ui.New(ui.ColorAuto, os.Stdout, os.Stderr).Error("%v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	global := globalFlags{color: string(ui.ColorAuto)}
	args, err := parseGlobal(args, &global)
	if err != nil {
		return err
	}
	renderer := ui.New(ui.ColorMode(global.color), os.Stdout, os.Stderr)
	renderer.SetVerbose(global.verbose)
	if len(args) == 0 {
		return commandInstall(renderer, nil)
	}

	switch args[0] {
	case "install":
		return commandInstall(renderer, args[1:])
	case "sync":
		return commandSync(renderer, args[1:])
	case "status":
		return commandStatus(renderer, args[1:])
	case "check-updates":
		return commandCheckUpdates(renderer, args[1:])
	case "update":
		return commandUpdate(renderer, args[1:])
	case "doctor":
		return commandDoctor(renderer, args[1:])
	case "uninstall":
		return commandUninstall(renderer, args[1:])
	case "hook":
		return commandHook(renderer, args[1:])
	case "version", "--version", "-version":
		fmt.Println(Version)
		return nil
	case "help", "--help", "-h":
		usage(os.Stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q; run elastic-docs-utils help", args[0])
	}
}

func parseGlobal(args []string, flags *globalFlags) ([]string, error) {
	remaining := make([]string, 0, len(args))
	for len(args) > 0 {
		if args[0] == "--verbose" {
			flags.verbose = true
			args = args[1:]
			continue
		}
		if !strings.HasPrefix(args[0], "--color") {
			remaining, args = append(remaining, args[0]), args[1:]
			continue
		}
		value := ""
		if args[0] == "--color" {
			if len(args) < 2 {
				return nil, errors.New("--color requires auto, always, or never")
			}
			value, args = args[1], args[2:]
		} else {
			value = strings.TrimPrefix(args[0], "--color=")
			args = args[1:]
		}
		if value != string(ui.ColorAuto) && value != string(ui.ColorAlways) && value != string(ui.ColorNever) {
			return nil, fmt.Errorf("invalid --color value %q; use auto, always, or never", value)
		}
		flags.color = value
	}
	return remaining, nil
}

func commandInstall(r *ui.Renderer, args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	hostList := fs.String("host", "", "comma-separated hosts")
	internal := fs.Bool("internal", false, "enable Elastic internal docs")
	yes := fs.Bool("yes", false, "non-interactive")
	dryRun := fs.Bool("dry-run", false, "show planned changes")
	force := fs.Bool("force", false, "replace managed conflicts and re-run selected tool installers")
	withVale := fs.Bool("with-vale", false, "install Vale and Elastic Vale rules")
	withDocsBuilder := fs.Bool("with-docs-builder", false, "install docs-builder")
	withDocsTools := fs.Bool("with-docs-tools", false, "install Vale, Elastic Vale rules, and docs-builder")
	noPrune := fs.Bool("no-prune", false, "keep skills that are no longer in the catalog")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r.Header(Version)
	if *dryRun {
		r.DryRun()
	}
	ids, err := requestedHosts(*hostList)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errors.New("no supported harnesses detected; install Claude Code, Codex, Cursor CLI, or OpenCode, or pass --host")
	}
	if *withDocsTools {
		*withVale = true
		*withDocsBuilder = true
	}
	r.Section("Configuring agent harnesses")
	rows := make([][]string, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, []string{string(id), "selected"})
	}
	r.Table([]string{"HOST", "STATUS"}, rows)
	// Vale and docs-builder are optional, and their installers touch system
	// locations that can fail for reasons unrelated to this setup. Report the
	// failure but still configure skills and host adapters, which are the
	// point of the command.
	toolFailures := installOptionalTools(r, *withVale, *withDocsBuilder, *dryRun, *force, *yes)
	reportToolFailures(r, toolFailures)

	if err := synchronize(ids, *internal, *dryRun, *force, !*noPrune, r); err != nil {
		return err
	}
	if !*dryRun {
		if err := state.SavePreferences(state.Preferences{Hosts: hosts.Strings(ids), Internal: *internal}); err != nil {
			return err
		}
		if path, err := paths.PreferencesPath(); err == nil {
			r.Verbose("Updated preferences: %s", path)
		}
		if err := refreshUpdates(r); err != nil {
			return err
		}
	}
	if *internal {
		r.Info("Elastic internal docs will be enabled when host adapters are installed.")
	}
	if *force {
		r.Warn("Replacing conflicting managed MCP entries with the Elastic Docs Utils configuration.")
	}
	if len(toolFailures) > 0 {
		return toolFailureError(toolFailures)
	}
	r.Success("Elastic Docs Utils is configured.")
	return nil
}

// installOptionalTools runs the selected upstream installers and returns every
// failure instead of stopping at the first one, so one broken installer cannot
// hide the state of the other.
func installOptionalTools(r *ui.Renderer, vale, docsBuilder, dryRun, force, assumeYes bool) []error {
	if !vale && !docsBuilder {
		return nil
	}
	r.Section("Installing documentation tools")
	if dryRun {
		if vale {
			r.Info("Would run the supported Elastic Vale Rules installer%s.", forced(force))
		}
		if docsBuilder {
			r.Info("Would run the supported docs-builder installer%s.", forced(force))
		}
		return nil
	}
	var failures []error
	if vale {
		r.Verbose("Runs the upstream Elastic Vale Rules installer; it reports the Vale binary, configuration, and rule paths it edits.")
		progress := r.StartProgress("Installing Vale and Elastic Vale rules%s", forced(force))
		err := bootstrap.InstallValeWithProgress(force, assumeYes, installerProgress(progress, "Vale and Elastic Vale rules"))
		progress.Stop()
		if err != nil {
			failures = append(failures, fmt.Errorf("install Vale and Elastic Vale rules: %w", err))
		} else {
			r.Success("Installed Vale and Elastic Vale rules.")
		}
	}
	if docsBuilder {
		r.Verbose("Runs the upstream docs-builder installer; it reports the binary path it edits.")
		progress := r.StartProgress("Preparing docs-builder installer%s", forced(force))
		callback := func(phase bootstrap.ProgressPhase, completed, total int64) {
			if phase == bootstrap.ProgressRunning {
				progress.Stop()
				r.Info("Running docs-builder installer; follow any prompts.")
				return
			}
			updateDownloadProgress(progress, "docs-builder", completed, total)
		}
		// The upstream installer exits 0 whether it installs, skips an existing
		// binary, or is declined at its overwrite prompt, so its exit code alone
		// cannot tell us what happened. Compare the version either side of it.
		before := updates.BinaryVersion("docs-builder", "--version")
		if err := bootstrap.InstallDocsBuilderWithProgress(force, assumeYes, callback); err != nil {
			failures = append(failures, fmt.Errorf("install docs-builder: %w", err))
		} else {
			progress.Stop()
			level, message := installerOutcome("docs-builder", before, updates.BinaryVersion("docs-builder", "--version"))
			switch level {
			case outcomeMissing:
				r.Warn("%s", message)
			case outcomeUnchanged:
				r.Info("%s", message)
			default:
				r.Success("%s", message)
			}
		}
		progress.Stop()
	}
	return failures
}

type installerOutcomeLevel int

const (
	outcomeInstalled installerOutcomeLevel = iota
	outcomeUpdated
	outcomeUnchanged
	outcomeMissing
)

// installerOutcome decides what an upstream installer actually did, from the
// tool's reported version either side of the run. The installers exit 0 whether
// they install, skip an existing binary, or are declined at an overwrite
// prompt, so a zero exit on its own must not be reported as an install.
func installerOutcome(tool, before, after string) (installerOutcomeLevel, string) {
	switch {
	case after == "":
		return outcomeMissing, fmt.Sprintf("The %s installer finished, but no %s binary is on PATH.", tool, tool)
	case before != "" && after == before:
		return outcomeUnchanged, fmt.Sprintf("%s left unchanged at %s; the installer did not replace it.", tool, after)
	case before == "":
		return outcomeInstalled, fmt.Sprintf("Installed %s %s.", tool, after)
	default:
		return outcomeUpdated, fmt.Sprintf("Updated %s %s to %s.", tool, before, after)
	}
}

func reportToolFailures(r *ui.Renderer, failures []error) {
	for _, failure := range failures {
		r.Warn("%v", failure)
	}
	if len(failures) > 0 {
		r.Warn("Continuing with the rest of the setup. Re-run the installer for the tools above once the cause is resolved.")
	}
}

func toolFailureError(failures []error) error {
	return fmt.Errorf("%d optional documentation tool installer(s) failed: %w", len(failures), errors.Join(failures...))
}

func forced(force bool) string {
	if force {
		return " (forcing replacement)"
	}
	return ""
}

func countedProgress(progress *ui.Progress) func(current, total int, label string) {
	if progress == nil {
		return nil
	}
	return func(current, total int, label string) {
		if total > 0 {
			progress.Update("[%d/%d] %s", current, total, label)
			return
		}
		progress.Update("%s", label)
	}
}

func installerProgress(progress *ui.Progress, component string) bootstrap.ProgressFunc {
	return func(phase bootstrap.ProgressPhase, completed, total int64) {
		if phase == bootstrap.ProgressRunning {
			progress.Update("Running %s installer", component)
			return
		}
		updateDownloadProgress(progress, component, completed, total)
	}
}

func updateDownloadProgress(progress *ui.Progress, component string, completed, total int64) {
	if total > 0 {
		percent := min(completed*100/total, 100)
		progress.Update("Downloading %s installer: %d%%", component, percent)
		return
	}
	if completed > 0 {
		if completed < 1024 {
			progress.Update("Downloading %s installer: %d bytes received", component, completed)
		} else {
			progress.Update("Downloading %s installer: %d KiB received", component, completed/1024)
		}
		return
	}
	progress.Update("Downloading %s installer", component)
}

func commandSync(r *ui.Renderer, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	hostList := fs.String("host", "", "comma-separated hosts")
	dryRun := fs.Bool("dry-run", false, "show planned changes")
	force := fs.Bool("force", false, "replace conflicting MCP entries")
	prune := fs.Bool("prune", true, "remove skills this tool installed that are no longer in the catalog")
	noPrune := fs.Bool("no-prune", false, "keep skills that are no longer in the catalog")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dryRun {
		r.DryRun()
	}
	prefs, err := state.LoadPreferences()
	if err != nil {
		return err
	}
	ids, err := hosts.Parse(*hostList)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		for _, name := range prefs.Hosts {
			ids = append(ids, hosts.ID(name))
		}
	}
	if len(ids) == 0 {
		return errors.New("nothing is installed; run elastic-docs-utils install first")
	}
	r.Header(Version)
	return synchronize(ids, prefs.Internal, *dryRun, *force, *prune && !*noPrune, r)
}

func commandStatus(r *ui.Renderer, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "machine-readable output")
	quiet := fs.Bool("quiet", false, "suppress normal output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	prefs, err := state.LoadPreferences()
	if err != nil {
		return err
	}
	cache, err := updates.Load()
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"version": Version, "hosts": prefs.Hosts, "internal": prefs.Internal, "updates": cache})
	}
	if *quiet {
		return nil
	}
	r.Header(Version)
	r.Section("Elastic Docs Utils status")
	rows := make([][]string, 0, len(prefs.Hosts))
	for _, host := range prefs.Hosts {
		rows = append(rows, []string{host, "configured"})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"—", "not installed"})
	}
	r.Table([]string{"HOST", "STATUS"}, rows)
	if !cache.CheckedAt.IsZero() {
		r.Section("Cached updates")
		updateRows := make([][]string, 0, len(cache.Items))
		for _, item := range cache.Items {
			updateRows = append(updateRows, []string{item.Name, item.State})
		}
		r.Table([]string{"COMPONENT", "STATUS"}, updateRows)
		renderUpdateHints(r, cache.Items)
	}
	return nil
}

func commandCheckUpdates(r *ui.Renderer, args []string) error {
	fs := flag.NewFlagSet("check-updates", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var progress *ui.Progress
	if !*asJSON {
		r.Header(Version)
		progress = r.StartProgress("Checking component versions")
	}
	status, err := updates.RefreshWithProgress(Version, countedProgress(progress))
	progress.Stop()
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(status)
	}
	renderUpdateStatus(r, status)
	return nil
}

func commandUpdate(r *ui.Renderer, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	components := fs.String("component", "all", "comma-separated components: elastic-docs-utils, skills, vale, vale-rules, docs-builder, or all")
	dryRun := fs.Bool("dry-run", false, "show planned changes")
	force := fs.Bool("force", false, "accept replacement prompts from upstream installers")
	noPrune := fs.Bool("no-prune", false, "keep skills that are no longer in the catalog")
	if err := fs.Parse(args); err != nil {
		return err
	}
	selected, err := parseUpdateComponents(*components)
	if err != nil {
		return err
	}
	if *dryRun {
		r.DryRun()
	}
	r.Header(Version)
	if selected.self {
		if *dryRun {
			r.Info("Would run the Elastic Docs Utils installer to update the binary.")
		} else {
			progress := r.StartProgress("Updating Elastic Docs Utils binary")
			err := bootstrap.SelfUpdateWithProgress(*force, true, installerProgress(progress, "Elastic Docs Utils"))
			progress.Stop()
			if err != nil {
				r.Warn("Could not self-update: %v", err)
			} else {
				r.Success("Updated Elastic Docs Utils binary.")
			}
		}
	} else {
		r.Info("Skipping Elastic Docs Utils binary.")
	}
	if selected.skills {
		prefs, err := state.LoadPreferences()
		if err != nil {
			return err
		}
		ids := make([]hosts.ID, len(prefs.Hosts))
		for i, host := range prefs.Hosts {
			ids[i] = hosts.ID(host)
		}
		if err := synchronize(ids, prefs.Internal, *dryRun, false, !*noPrune, r); err != nil {
			return err
		}
	} else {
		r.Info("Skipping Elastic Docs skills.")
	}
	// A failed component must not stop the remaining ones, and the refreshed
	// status below is most useful precisely when something went wrong.
	var toolFailures []error
	if !selected.vale {
		r.Info("Skipping Vale and Elastic Vale rules.")
	}
	if !selected.docsBuilder {
		r.Info("Skipping docs-builder.")
	}
	// One call for both tools: installOptionalTools prints its own section
	// header, so calling it per tool printed "Installing documentation tools"
	// twice and read like two separate phases.
	toolFailures = append(toolFailures, installOptionalTools(r, selected.vale, selected.docsBuilder, *dryRun, *force, false)...)
	reportToolFailures(r, toolFailures)
	if *dryRun {
		r.Info("Would refresh documentation tool status after updates.")
		return nil
	}
	if err := refreshUpdates(r); err != nil {
		return err
	}
	if len(toolFailures) > 0 {
		return toolFailureError(toolFailures)
	}
	return nil
}

type updateComponents struct {
	self        bool
	skills      bool
	vale        bool
	docsBuilder bool
}

func parseUpdateComponents(value string) (updateComponents, error) {
	if strings.TrimSpace(value) == "" {
		return updateComponents{}, errors.New("component list cannot be empty")
	}
	var selected updateComponents
	for _, raw := range strings.Split(value, ",") {
		switch component := strings.TrimSpace(raw); component {
		case "all":
			selected = updateComponents{self: true, skills: true, vale: true, docsBuilder: true}
		case "elastic-docs-utils":
			selected.self = true
		case "skills":
			selected.skills = true
		case "vale", "vale-rules":
			selected.vale = true
		case "docs-builder":
			selected.docsBuilder = true
		default:
			return updateComponents{}, fmt.Errorf("unknown update component %q; valid components: elastic-docs-utils, skills, vale, vale-rules, docs-builder, all", component)
		}
	}
	return selected, nil
}

func refreshUpdates(r *ui.Renderer) error {
	progress := r.StartProgress("Refreshing component status")
	status, err := updates.RefreshWithProgress(Version, countedProgress(progress))
	progress.Stop()
	if err != nil {
		return err
	}
	renderUpdateStatus(r, status)
	return nil
}

func renderUpdateStatus(r *ui.Renderer, status updates.Status) {
	r.Section("Update status")
	rows := make([][]string, 0, len(status.Items))
	for _, item := range status.Items {
		rows = append(rows, []string{item.Name, item.Installed, item.Latest, item.State})
	}
	r.Table([]string{"COMPONENT", "INSTALLED", "LATEST", "STATUS"}, rows)
	renderUpdateHints(r, status.Items)
}

// renderUpdateHints prints the next step for each row that is not current. A
// status of "missing" or "unknown" is not actionable on its own.
func renderUpdateHints(r *ui.Renderer, items []updates.Item) {
	for _, item := range items {
		if item.Hint != "" && item.State != "current" && item.State != "local" {
			r.Info("%s: %s", item.Name, item.Hint)
		}
	}
}

func commandDoctor(r *ui.Renderer, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	detected := hosts.Detect()
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"version": Version, "detectedHosts": detected})
	}
	r.Header(Version)
	r.Section("Detected harnesses")
	rows := make([][]string, 0, len(hosts.All()))
	found := map[hosts.ID]bool{}
	for _, host := range detected {
		found[host.ID] = true
	}
	for _, host := range hosts.All() {
		status := "not found"
		if found[host.ID] {
			status = "available"
		}
		rows = append(rows, []string{host.Label, status})
	}
	r.Table([]string{"HOST", "STATUS"}, rows)
	return nil
}

func commandUninstall(r *ui.Renderer, args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	purge := fs.Bool("purge", false, "remove owned state")
	dryRun := fs.Bool("dry-run", false, "show planned changes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dryRun {
		r.DryRun()
	}
	s, err := state.Load()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(s.Skills))
	for name := range s.Skills {
		names = append(names, name)
	}
	skipped, err := skills.RemoveOwned(names, *dryRun)
	if err != nil {
		return err
	}
	for _, name := range skipped {
		r.Warn("Left %s in place: it was not installed by this tool.", name)
	}
	if *dryRun {
		r.Info("Would remove %d managed skills. Existing MCP configuration is retained for safety.", len(names))
		return nil
	}
	if *purge {
		for _, path := range []func() (string, error){paths.StatePath, paths.PreferencesPath} {
			value, err := path()
			if err != nil {
				return err
			}
			if err := os.Remove(value); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		r.Success("Removed managed skills and local Elastic Docs Utils state.")
	} else {
		s.Skills = map[string]state.SkillState{}
		if err := state.Save(s); err != nil {
			return err
		}
		r.Success("Removed managed skills. MCP entries were retained for safety; use --purge to remove local state.")
	}
	return nil
}

func commandHook(r *ui.Renderer, args []string) error {
	if len(args) != 3 || args[0] != "session-start" || args[1] != "--host" {
		return errors.New("usage: elastic-docs-utils hook session-start --host <host>")
	}
	_, err := hosts.Parse(args[2])
	if err != nil {
		return err
	}
	// Hooks are intentionally cache-only: no shell-start latency or networking.
	cache, err := updates.Load()
	if err != nil || cache.CheckedAt.IsZero() {
		return nil
	}
	for _, item := range cache.Items {
		if item.State == "update available" {
			fmt.Fprintf(os.Stdout, "%s update available. %s\n", item.Name, item.Hint)
		}
	}
	return nil
}

// reportPrune removes, or on a dry run reports, the skills that state records
// as installed from an active catalog but that the catalog no longer holds. A
// directory this tool did not install is reported and kept, and its state
// record is kept too so the warning repeats until someone acts on it.
func reportPrune(r *ui.Renderer, s *state.State, current map[string]state.SkillState, internal, dryRun bool) error {
	stale := skills.Stale(current, s.Skills, skills.ActiveRepos(internal))
	if len(stale) == 0 {
		return nil
	}
	skipped, err := skills.RemoveOwned(stale, dryRun)
	if err != nil {
		return err
	}
	unmanaged := make(map[string]bool, len(skipped))
	for _, name := range skipped {
		unmanaged[name] = true
		r.Warn("Left %s in place: it is no longer in the catalog but was not installed by this tool. Remove it yourself if you no longer want it.", name)
	}
	for _, name := range stale {
		if unmanaged[name] {
			continue
		}
		if dryRun {
			r.Info("Would prune removed skill: %s", name)
			continue
		}
		delete(s.Skills, name)
		r.Info("Pruned removed skill: %s", name)
	}
	return nil
}

func synchronize(ids []hosts.ID, internal, dryRun, force, prune bool, r *ui.Renderer) error {
	s, err := state.Load()
	if err != nil {
		return err
	}
	if !s.MigratedLegacy {
		migrated, err := adapters.MigrateLegacy(dryRun)
		if err != nil {
			return err
		}
		if migrated {
			if dryRun {
				r.Info("Would remove retired docs-harness hook entries while preserving other Claude settings.")
			} else {
				r.Info("Removed retired docs-harness hook entries while preserving other Claude settings.")
			}
		}
		if !dryRun {
			s.MigratedLegacy = true
		}
	}
	r.Section("Synchronizing shared skills")
	var skillProgress *ui.Progress
	if !dryRun {
		skillProgress = r.StartProgress("Fetching and installing skill catalogs")
	}
	skillResult, err := skills.SyncWithProgress(ids, internal, dryRun, countedProgress(skillProgress))
	skillProgress.Stop()
	if err != nil {
		return err
	}
	for _, name := range skillResult.Adopted {
		if dryRun {
			r.Info("Would adopt and refresh catalog skill: %s", name)
			continue
		}
		r.Info("Adopted and refreshed catalog skill: %s", name)
	}
	for _, name := range skillResult.Skipped {
		if dryRun {
			r.Warn("Would skip catalog skill %s because an existing directory was not installed by this tool.", name)
			continue
		}
		delete(s.Skills, name)
		r.Warn("Skipped catalog skill %s because an existing directory was not installed by this tool. The existing directory was left unchanged.", name)
	}
	if dryRun {
		r.Info("Would refresh public%s skills in ~/.agents/skills.", map[bool]string{true: " and internal", false: ""}[internal])
		if root, err := paths.CanonicalSkillsDir(); err == nil {
			r.Verbose("Would refresh managed skills beneath: %s", root)
		}
	} else {
		r.Success("Installed %d managed skills.", len(skillResult.Installed))
		for _, path := range skillResult.Files {
			r.Verbose("Updated managed skill or discovery link: %s", path)
		}
	}
	r.Section("Synchronizing host adapters")
	self, err := bootstrap.EnsureSelfInstalled(dryRun)
	if err != nil {
		return err
	}
	if self.Installed {
		r.Info("Installed Elastic Docs Utils to %s.", self.Path)
		r.Verbose("Installed Elastic Docs Utils binary: %s", self.Path)
	} else if dryRun {
		r.Verbose("Would install Elastic Docs Utils binary: %s", self.Path)
	}
	var adapterProgress *ui.Progress
	if !dryRun {
		adapterProgress = r.StartProgress("Configuring selected host adapters")
	}
	adapterResult, adapterErr := adapters.SyncWithProgress(ids, internal, dryRun, force, self.Path, countedProgress(adapterProgress))
	adapterProgress.Stop()
	// A host adapter that fails to configure must not strand the skill work that
	// already succeeded. Returning here skipped both the state save and the
	// prune, so one broken adapter silently left stale skills on disk and the
	// state file describing a sync that did not finish. Report it, finish the
	// skill side, and surface the failure at the end.
	if adapterErr != nil {
		r.Warn("Could not configure host adapters: %v", adapterErr)
	}
	if dryRun {
		for name, host := range adapterResult.Hosts {
			for _, path := range host.Files {
				r.Verbose("Would update %s configuration: %s", name, path)
			}
		}
		r.Info("Would configure %d selected host adapters.", len(ids))
		// Pruning is on by default, so a dry run has to show what it would
		// delete. RemoveOwned writes nothing here; it only classifies.
		if prune {
			if err := reportPrune(r, &s, skillResult.Catalog, internal, true); err != nil {
				return err
			}
		}
		return adapterErr
	}
	for name, host := range adapterResult.Hosts {
		s.Hosts[name] = host
		for _, path := range host.Files {
			r.Verbose("Updated %s configuration: %s", name, path)
		}
	}
	for name, record := range skillResult.Records {
		s.Skills[name] = record
	}
	if prune {
		if err := reportPrune(r, &s, skillResult.Catalog, internal, false); err != nil {
			return err
		}
	}
	if err := state.Save(s); err != nil {
		return err
	}
	if path, err := paths.StatePath(); err == nil {
		r.Verbose("Updated managed state: %s", path)
	}
	for _, warning := range adapterResult.Warnings {
		r.Warn("%s", warning)
	}
	if len(adapterResult.Validated) > 0 {
		r.Success("Validated MCP configuration for %d host adapters.", len(adapterResult.Validated))
	}
	if adapterErr != nil {
		return adapterErr
	}
	r.Success("Synchronized %d host adapters.", len(ids))
	return nil
}

func requestedHosts(value string) ([]hosts.ID, error) {
	ids, err := hosts.Parse(value)
	if err != nil || len(ids) > 0 {
		return ids, err
	}
	for _, host := range hosts.Detect() {
		ids = append(ids, host.ID)
	}
	return ids, nil
}

func usage(out *os.File) {
	fmt.Fprint(out, `Elastic Docs Utils

Usage:
  elastic-docs-utils <command> [options]

Commands:
  install        Configure detected coding-agent harnesses and optional tools
  sync           Reconcile selected host integrations
  status         Show configuration and cached update status
  check-updates  Refresh update status
  update         Apply selected updates explicitly
  doctor         Diagnose available harnesses
  uninstall      Remove owned integrations
  version        Print the version

Global options:
  --color auto|always|never
  --verbose                 Show managed file locations that are changed
`)
}
