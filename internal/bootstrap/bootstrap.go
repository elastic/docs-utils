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

// Package bootstrap runs the platform installers maintained by the upstream
// documentation-tool projects.
package bootstrap

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	valeRulesRaw       = "https://raw.githubusercontent.com/elastic/vale-rules/main/"
	docsBuilderUnix    = "https://ela.st/docs-builder-install"
	docsBuilderWindows = "https://ela.st/docs-builder-install-win"
)

// InstallVale delegates to the official Elastic Vale Rules installer, which
// installs the Vale binary when necessary and installs the Elastic rule bundle.
// Force confirms replacement of an existing non-Elastic Vale configuration.
func InstallVale(force bool) error {
	name, shell, err := valeScript()
	if err != nil {
		return err
	}
	return downloadAndRun(valeRulesRaw+name, shell, force)
}

// InstallDocsBuilder delegates to the official Docs Builder installer. Force
// confirms replacement when the installer finds an existing binary.
func InstallDocsBuilder(force bool) error {
	if runtime.GOOS == "windows" {
		return downloadAndRun(docsBuilderWindows, "powershell", force)
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("docs-builder installation is not supported on %s", runtime.GOOS)
	}
	return downloadAndRun(docsBuilderUnix, "sh", force)
}

func valeScript() (string, string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "install-macos.sh", "bash", nil
	case "linux":
		return "install-linux.sh", "bash", nil
	case "windows":
		return "install-windows.ps1", "powershell", nil
	default:
		return "", "", fmt.Errorf("Vale installation is not supported on %s", runtime.GOOS)
	}
}

func downloadAndRun(url, shell string, force bool) error {
	path, err := download(url, extension(shell))
	if err != nil {
		return err
	}
	defer os.Remove(path)
	command, args, err := runner(shell, path)
	if err != nil {
		return err
	}
	cmd := exec.Command(command, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if force {
		// The maintained installers ask only before replacing existing local
		// configuration or binaries. Supplying yes lets --force be safely
		// non-interactive without exposing configuration contents.
		cmd.Stdin = strings.NewReader("y\n")
	} else {
		cmd.Stdin = os.Stdin
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run upstream installer: %w", err)
	}
	return nil
}

func download(url, suffix string) (string, error) {
	client := http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download installer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download installer: %s", resp.Status)
	}
	file, err := os.CreateTemp("", "elastic-docs-utils-installer-*"+suffix)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err := io.Copy(file, resp.Body); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func extension(shell string) string {
	if shell == "powershell" {
		return ".ps1"
	}
	return ".sh"
}

func runner(shell, path string) (string, []string, error) {
	switch shell {
	case "bash", "sh":
		return shell, []string{path}, nil
	case "powershell":
		for _, candidate := range []string{"powershell", "powershell.exe", "pwsh"} {
			if command, err := exec.LookPath(candidate); err == nil {
				return command, []string{"-ExecutionPolicy", "Bypass", "-File", filepath.Clean(path)}, nil
			}
		}
		return "", nil, fmt.Errorf("PowerShell is required to run the installer")
	default:
		return "", nil, fmt.Errorf("unsupported installer shell %q", shell)
	}
}
