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

package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestWordmarkForWidth(t *testing.T) {
	if got := wordmarkForWidth(74); got != compactWordmark {
		t.Fatal("narrow terminal did not receive compact wordmark")
	}
	if got := wordmarkForWidth(120); got != wordmark {
		t.Fatal("wide terminal did not receive full wordmark")
	}
}

func TestProgressUsesDurableLineForRedirectedOutput(t *testing.T) {
	var out bytes.Buffer
	r := New(ColorNever, &out, &out)
	progress := r.StartProgress("Checking component versions")
	progress.Update("[1/5] Checking Elastic Docs Utils")
	progress.Stop()
	progress.Stop()

	want := "[INFO] Checking component versions...\n[INFO] [1/5] Checking Elastic Docs Utils\n"
	if got := out.String(); got != want {
		t.Fatalf("progress output = %q", got)
	}
}

func TestProgressClearsInteractiveLine(t *testing.T) {
	var out bytes.Buffer
	r := &Renderer{out: &out, err: &out, interactive: true}
	progress := r.StartProgress("Fetching skills")
	progress.Stop()

	if got := out.String(); !strings.Contains(got, "[|] Fetching skills") || !strings.HasSuffix(got, "\r\033[2K") {
		t.Fatalf("interactive progress was not drawn and cleared: %q", got)
	}
}
