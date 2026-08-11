package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/elastic/docs-utils/internal/bootstrap"
	"github.com/elastic/docs-utils/internal/ui"
	"github.com/elastic/docs-utils/internal/updates"
)

func TestParseUpdateComponents(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  updateComponents
	}{
		{
			name:  "all by default",
			value: "all",
			want:  updateComponents{self: true, skills: true, vale: true, docsBuilder: true},
		},
		{
			name:  "selected components",
			value: "skills,vale-rules,docs-builder",
			want:  updateComponents{skills: true, vale: true, docsBuilder: true},
		},
		{
			name:  "vale aliases the rules installer",
			value: "vale",
			want:  updateComponents{vale: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseUpdateComponents(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("components = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseUpdateComponentsRejectsUnknownComponent(t *testing.T) {
	if _, err := parseUpdateComponents("skills,unknown"); err == nil {
		t.Fatal("unknown component was accepted")
	}
}

func TestInstallOptionalToolsSkippedWhenNoneSelected(t *testing.T) {
	var out bytes.Buffer
	r := ui.New(ui.ColorNever, &out, &out)
	if failures := installOptionalTools(r, false, false, false, false, false); failures != nil {
		t.Fatalf("failures = %v, want none", failures)
	}
	if out.Len() != 0 {
		t.Fatalf("unselected tools produced output: %q", out.String())
	}
}

func TestProgressCallbacksRenderCountsAndPercentages(t *testing.T) {
	var out bytes.Buffer
	r := ui.New(ui.ColorNever, &out, &out)
	progress := r.StartProgress("Working")
	countedProgress(progress)(2, 5, "Checking Vale")
	updateDownloadProgress(progress, "docs-builder", 25, 100)
	installerProgress(progress, "docs-builder")(bootstrap.ProgressRunning, 0, 0)
	progress.Stop()

	for _, want := range []string{"[2/5] Checking Vale", "Downloading docs-builder installer: 25%", "Running docs-builder installer"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("progress output missing %q:\n%s", want, out.String())
		}
	}
}

func TestInstallOptionalToolsDryRunRunsNoInstaller(t *testing.T) {
	var out bytes.Buffer
	r := ui.New(ui.ColorNever, &out, &out)
	if failures := installOptionalTools(r, true, true, true, false, false); failures != nil {
		t.Fatalf("failures = %v, want none", failures)
	}
	for _, want := range []string{"Would run the supported Elastic Vale Rules installer", "Would run the supported docs-builder installer"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("dry run output missing %q:\n%s", want, out.String())
		}
	}
}

func TestToolFailureErrorKeepsEveryCause(t *testing.T) {
	vale := errors.New("install Vale and Elastic Vale rules: boom")
	builder := errors.New("install docs-builder: boom")
	err := toolFailureError([]error{vale, builder})
	if !errors.Is(err, vale) || !errors.Is(err, builder) {
		t.Fatalf("aggregated error lost a cause: %v", err)
	}
	if !strings.Contains(err.Error(), "2 optional") {
		t.Fatalf("error = %q, want the failure count", err.Error())
	}
}

func TestReportToolFailuresWarnsAndContinues(t *testing.T) {
	var out bytes.Buffer
	r := ui.New(ui.ColorNever, &out, &out)
	reportToolFailures(r, []error{errors.New("install docs-builder: boom")})
	if !strings.Contains(out.String(), "install docs-builder: boom") {
		t.Fatalf("failure was not reported:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Continuing with the rest of the setup") {
		t.Fatalf("output does not say setup continues:\n%s", out.String())
	}
}

func TestRenderUpdateStatusShowsHintsForActionableRows(t *testing.T) {
	var out bytes.Buffer
	r := ui.New(ui.ColorNever, &out, &out)
	renderUpdateStatus(r, updates.Status{Items: []updates.Item{
		{Name: "docs-builder", Installed: "not installed", State: "missing", Hint: "Run `elastic-docs-utils install --with-docs-builder`"},
		{Name: "Vale", Installed: "3.17.0", Latest: "3.17.0", State: "current", Hint: "should not appear"},
	}})
	if !strings.Contains(out.String(), "Run `elastic-docs-utils install --with-docs-builder`") {
		t.Fatalf("missing component has no next step:\n%s", out.String())
	}
	if strings.Contains(out.String(), "should not appear") {
		t.Fatalf("current component printed a hint:\n%s", out.String())
	}
}
