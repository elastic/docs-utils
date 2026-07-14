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

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtension(t *testing.T) {
	if got := extension("powershell"); got != ".ps1" {
		t.Fatalf("PowerShell extension = %q", got)
	}
	if got := extension("bash"); got != ".sh" {
		t.Fatalf("shell extension = %q", got)
	}
}

func TestSamePath(t *testing.T) {
	path := filepath.Join("/tmp", "elastic-docs-utils")
	if !samePath(path, path) {
		t.Fatal("identical paths differ")
	}
}

func TestCopyExecutable(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "bin", binaryName)
	if err := os.WriteFile(source, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyExecutable(source, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "binary" {
		t.Fatalf("copied binary = %q", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o, want 755", info.Mode().Perm())
	}
}
