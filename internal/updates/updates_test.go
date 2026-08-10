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

package updates

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elastic/docs-utils/internal/state"
)

func TestSemverGT(t *testing.T) {
	for _, test := range []struct {
		a, b string
		want bool
	}{
		{"2.0.0", "1.9.9", true},
		{"1.2.1", "1.2.0", true},
		{"1.2.0", "1.2.0", false},
		{"1.2.0", "1.3.0", false},
	} {
		if got := semverGT(test.a, test.b); got != test.want {
			t.Errorf("semverGT(%q, %q) = %t, want %t", test.a, test.b, got, test.want)
		}
	}
}

func TestElasticDocsUtilsLocalBuild(t *testing.T) {
	item := checkElasticDocsUtils("dev")
	if item.State != "local" || item.Installed != "local build" {
		t.Fatalf("local build item = %#v", item)
	}
}

func TestSkillStatus(t *testing.T) {
	commit := "1234567890abcdef"
	records := map[string]state.SkillState{
		"write-docs": {Source: "https://github.com/elastic/elastic-docs-skills.git", Commit: commit},
	}
	if item := repoSkillStatus("Elastic Docs skills", publicSkillsSource, records, commit); item.State != "current" || item.Installed != "1234567890ab" {
		t.Fatalf("current skill item = %#v", item)
	}
	if item := repoSkillStatus("Elastic Docs skills", publicSkillsSource, records, "abcdef1234567890"); item.State != "update available" {
		t.Fatalf("outdated skill item = %#v", item)
	}
}

func TestParseVersionPrefersVersionOnlyLine(t *testing.T) {
	// A tool that logs before reporting its version must not be misread as
	// whichever version-shaped string appears first in the log.
	out := "info ::config:: loaded schema 2.0.1 from cache\n1.32.0+b21afa632aad\n"
	if got := parseVersion(out); got != "1.32.0" {
		t.Fatalf("parseVersion = %q, want 1.32.0", got)
	}
}

func TestParseVersionFallsBackToInlineMatch(t *testing.T) {
	if got := parseVersion("vale version 3.17.0\n"); got != "3.17.0" {
		t.Fatalf("parseVersion = %q, want 3.17.0", got)
	}
	if got := parseVersion("v2.2.0"); got != "2.2.0" {
		t.Fatalf("parseVersion = %q, want 2.2.0", got)
	}
}

func TestCompareReportsInstallHintWhenMissing(t *testing.T) {
	item := compare("docs-builder", "", "1.32.0", nil, hints{missing: "install it", update: "update it"})
	if item.State != "missing" || item.Installed != "not installed" {
		t.Fatalf("missing item = %#v", item)
	}
	if item.Hint != "install it" {
		t.Fatalf("hint = %q, want the install hint", item.Hint)
	}
}

func TestCompareStates(t *testing.T) {
	h := hints{missing: "install it", update: "update it"}
	if item := compare("tool", "1.0.0", "2.0.0", nil, h); item.State != "update available" || item.Hint != "update it" {
		t.Fatalf("outdated item = %#v", item)
	}
	if item := compare("tool", "2.0.0", "2.0.0", nil, h); item.State != "current" || item.Hint != "" {
		t.Fatalf("current item = %#v", item)
	}
}

func TestCompareDistinguishesLookupFailureFromMissingTool(t *testing.T) {
	h := hints{missing: "install it", update: "update it"}
	item := compare("tool", "1.0.0", "", errRateLimited, h)
	if item.State != "unknown" {
		t.Fatalf("state = %q, want unknown", item.State)
	}
	if !strings.Contains(item.Hint, "rate limit") {
		t.Fatalf("hint = %q, want a rate limit explanation", item.Hint)
	}
}

func TestRateLimited(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Remaining", "0")
	if !rateLimited(resp) {
		t.Fatal("exhausted rate limit was not detected")
	}
	resp.Header.Set("X-RateLimit-Remaining", "12")
	if rateLimited(resp) {
		t.Fatal("forbidden response with quota left reported as rate limited")
	}
}

func TestRateLimitedTreats429AsRateLimited(t *testing.T) {
	// GitHub's secondary rate limit returns 429 without X-RateLimit-Remaining.
	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
	if !rateLimited(resp) {
		t.Fatal("429 without X-RateLimit-Remaining was not detected as rate limited")
	}
}

func TestCompareHintWhenLatestUnparseable(t *testing.T) {
	h := hints{missing: "install it", update: "update it"}
	item := compare("tool", "1.0.0", "", nil, h)
	if item.State != "unknown" {
		t.Fatalf("state = %q, want unknown", item.State)
	}
	if item.Hint == "" {
		t.Fatalf("hint is empty; want an explanation for the unknown state")
	}
}

func TestGithubJSONSendsToken(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"tag_name":"v1.2.3"}`)
	}))
	defer server.Close()
	t.Setenv("GITHUB_TOKEN", "secret")

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := githubJSON(server.URL, &payload); err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer secret" {
		t.Fatalf("Authorization = %q, want the token", authorization)
	}
	if payload.TagName != "v1.2.3" {
		t.Fatalf("tag = %q", payload.TagName)
	}
}

func TestGithubJSONReportsRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	var payload struct{}
	if err := githubJSON(server.URL, &payload); err != errRateLimited {
		t.Fatalf("err = %v, want errRateLimited", err)
	}
}
