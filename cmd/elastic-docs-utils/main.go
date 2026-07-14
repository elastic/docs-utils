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
	force := fs.Bool("force", false, "replace owned conflicts")
	withVale := fs.Bool("with-vale", false, "install Vale and Elastic Vale rules")
	withDocsBuilder := fs.Bool("with-docs-builder", false, "install docs-builder")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = yes
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
	r.Section("Configuring agent harnesses")
	rows := make([][]string, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, []string{string(id), "selected"})
	}
	r.Table([]string{"HOST", "STATUS"}, rows)
	if err := installOptionalTools(r, *withVale, *withDocsBuilder, *dryRun); err != nil {
		return err
	}

	if err := synchronize(ids, *internal, *dryRun, *force, r); err != nil {
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
	r.Success("Elastic Docs Utils is configured.")
	return nil
}

func installOptionalTools(r *ui.Renderer, vale, docsBuilder, dryRun bool) error {
	if !vale && !docsBuilder {
		return nil
	}
	r.Section("Installing documentation tools")
	if dryRun {
		if vale {
			r.Info("Would run the supported Elastic Vale Rules installer.")
		}
		if docsBuilder {
			r.Info("Would run the supported docs-builder installer.")
		}
		return nil
	}
	if vale {
		r.Info("Installing Vale and Elastic Vale rules.")
		r.Verbose("Runs the upstream Elastic Vale Rules installer; it reports the Vale binary, configuration, and rule paths it edits.")
		if err := bootstrap.InstallVale(); err != nil {
			return fmt.Errorf("install Vale and Elastic Vale rules: %w", err)
		}
	}
	if docsBuilder {
		r.Info("Installing docs-builder.")
		r.Verbose("Runs the upstream docs-builder installer; it reports the binary path it edits.")
		if err := bootstrap.InstallDocsBuilder(); err != nil {
			return fmt.Errorf("install docs-builder: %w", err)
		}
	}
	return nil
}

func commandSync(r *ui.Renderer, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	hostList := fs.String("host", "", "comma-separated hosts")
	dryRun := fs.Bool("dry-run", false, "show planned changes")
	force := fs.Bool("force", false, "replace conflicting MCP entries")
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
	return synchronize(ids, prefs.Internal, *dryRun, *force, r)
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
	status, err := updates.Refresh(Version)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(status)
	}
	r.Header(Version)
	renderUpdateStatus(r, status)
	return nil
}

func commandUpdate(r *ui.Renderer, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	components := fs.String("component", "skills", "comma-separated components")
	dryRun := fs.Bool("dry-run", false, "show planned changes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	selected := strings.Split(*components, ",")
	for _, component := range selected {
		if strings.TrimSpace(component) != "skills" {
			return fmt.Errorf("%s updates are managed by its own installer; use `elastic-docs-utils check-updates` for status", strings.TrimSpace(component))
		}
	}
	if *dryRun {
		r.DryRun()
	}
	prefs, err := state.LoadPreferences()
	if err != nil {
		return err
	}
	ids := make([]hosts.ID, len(prefs.Hosts))
	for i, host := range prefs.Hosts {
		ids[i] = hosts.ID(host)
	}
	r.Header(Version)
	if err := synchronize(ids, prefs.Internal, *dryRun, false, r); err != nil {
		return err
	}
	if *dryRun {
		r.Info("Would refresh documentation tool status after synchronization.")
		return nil
	}
	return refreshUpdates(r)
}

func refreshUpdates(r *ui.Renderer) error {
	status, err := updates.Refresh(Version)
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
	if err := skills.RemoveOwned(names, *dryRun); err != nil {
		return err
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

func synchronize(ids []hosts.ID, internal, dryRun, force bool, r *ui.Renderer) error {
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
	skillResult, err := skills.Sync(ids, internal, dryRun)
	if err != nil {
		return err
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
	adapterResult, err := adapters.Sync(ids, internal, dryRun, force)
	if err != nil {
		return err
	}
	if dryRun {
		for name, host := range adapterResult.Hosts {
			for _, path := range host.Files {
				r.Verbose("Would update %s configuration: %s", name, path)
			}
		}
		r.Info("Would configure %d selected host adapters.", len(ids))
		return nil
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
