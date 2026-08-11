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
	"bytes"
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
	selfInstallerUnix  = "https://ela.st/docs-utils-sh"
	selfInstallerWin   = "https://ela.st/docs-utils-ps"
	binaryName         = "elastic-docs-utils"
)

// ProgressPhase identifies the measurable download phase and the opaque
// upstream-installer phase of a bootstrap operation.
type ProgressPhase string

const (
	ProgressDownloading ProgressPhase = "downloading"
	ProgressRunning     ProgressPhase = "running"
)

// ProgressFunc reports bytes while downloading an installer script. Total is
// zero when the server does not provide a content length. The running phase
// has no numeric total because upstream installers do not expose one.
type ProgressFunc func(phase ProgressPhase, completed, total int64)

// SelfInstall describes the stable executable location selected for the
// current binary.
type SelfInstall struct {
	Path      string
	Installed bool
}

// EnsureSelfInstalled copies a binary started from a checkout or download to
// a durable executable location. It prefers /usr/local/bin when writable and
// otherwise uses the user's ~/.local/bin directory. Windows uses LocalAppData.
// The returned path is suitable for long-lived host hooks.
func EnsureSelfInstalled(dryRun bool) (SelfInstall, error) {
	current, err := os.Executable()
	if err != nil {
		return SelfInstall{}, fmt.Errorf("locate Elastic Docs Utils executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(current); err == nil {
		current = resolved
	}
	targets, err := selfInstallTargets()
	if err != nil {
		return SelfInstall{}, err
	}
	for _, target := range targets {
		if samePath(current, target) {
			return SelfInstall{Path: target}, nil
		}
	}
	if dryRun {
		return SelfInstall{Path: targets[0]}, nil
	}
	for _, target := range targets {
		if err := copyExecutable(current, target); err == nil {
			return SelfInstall{Path: target, Installed: true}, nil
		}
	}
	return SelfInstall{}, fmt.Errorf("install Elastic Docs Utils to a durable binary directory")
}

func selfInstallTargets() ([]string, error) {
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			return nil, fmt.Errorf("LOCALAPPDATA is not set")
		}
		return []string{filepath.Join(localAppData, "Elastic", "DocsUtils", binaryName+".exe")}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return []string{
		filepath.Join("/usr/local/bin", binaryName),
		filepath.Join(home, ".local", "bin", binaryName),
	}, nil
}

func samePath(left, right string) bool {
	if resolved, err := filepath.EvalSymlinks(left); err == nil {
		left = resolved
	}
	if resolved, err := filepath.EvalSymlinks(right); err == nil {
		right = resolved
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func copyExecutable(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary, err := os.CreateTemp(filepath.Dir(target), ".elastic-docs-utils-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, input); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o755); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, target); err == nil {
		return nil
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, target)
}

// SelfUpdate downloads and runs the official elastic-docs-utils installer,
// replacing the running binary in-place. It is a no-op on unsupported platforms.
func SelfUpdate(force, assumeYes bool) error {
	return SelfUpdateWithProgress(force, assumeYes, nil)
}

// SelfUpdateWithProgress runs SelfUpdate with download and execution updates.
func SelfUpdateWithProgress(force, assumeYes bool, progress ProgressFunc) error {
	switch runtime.GOOS {
	case "darwin", "linux":
		return downloadAndRun(selfInstallerUnix, "sh", force, assumeYes, true, progress)
	case "windows":
		return downloadAndRun(selfInstallerWin, "powershell", force, assumeYes, true, progress)
	default:
		return fmt.Errorf("self-update is not supported on %s; download the binary from https://github.com/elastic/docs-utils/releases", runtime.GOOS)
	}
}

// InstallVale delegates to the official Elastic Vale Rules installer, which
// installs the Vale binary when necessary and installs the Elastic rule bundle.
// Force confirms replacement of an existing non-Elastic Vale configuration.
// AssumeYes keeps the installer from blocking on a prompt it cannot read.
func InstallVale(force, assumeYes bool) error {
	return InstallValeWithProgress(force, assumeYes, nil)
}

// InstallValeWithProgress runs InstallVale with download and execution updates.
func InstallValeWithProgress(force, assumeYes bool, progress ProgressFunc) error {
	name, shell, err := valeScript()
	if err != nil {
		return err
	}
	return downloadAndRun(valeRulesRaw+name, shell, force, assumeYes, true, progress)
}

// InstallDocsBuilder delegates to the official Docs Builder installer. Force
// confirms replacement when the installer finds an existing binary. AssumeYes
// keeps the installer from blocking on a prompt it cannot read.
// The installer is interactive: it asks for permission and may prompt for a
// root password, so its output is always passed through to the terminal.
func InstallDocsBuilder(force, assumeYes bool) error {
	return InstallDocsBuilderWithProgress(force, assumeYes, nil)
}

// InstallDocsBuilderWithProgress runs InstallDocsBuilder with download and
// execution updates. Once execution starts, the upstream output is streamed.
func InstallDocsBuilderWithProgress(force, assumeYes bool, progress ProgressFunc) error {
	if runtime.GOOS == "windows" {
		return downloadAndRun(docsBuilderWindows, "powershell", force, assumeYes, false, progress)
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("docs-builder installation is not supported on %s", runtime.GOOS)
	}
	return downloadAndRun(docsBuilderUnix, "sh", force, assumeYes, false, progress)
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

// downloadAndRun fetches a script from url and executes it with shell.
// When quiet is true, stdout and stderr are captured and only surfaced on
// failure; when false (interactive installers such as docs-builder), they
// pass through to the terminal so the user can respond to prompts.
func downloadAndRun(url, shell string, force, assumeYes, quiet bool, progress ProgressFunc) error {
	path, err := download(url, extension(shell), progress)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	command, args, err := runner(shell, path)
	if err != nil {
		return err
	}
	cmd := exec.Command(command, args...)
	cmd.Stdin = installerInput(force, assumeYes)
	var out bytes.Buffer
	if quiet {
		cmd.Stdout = &out
		cmd.Stderr = &out
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	reportProgress(progress, ProgressRunning, 0, 0)
	if err := cmd.Run(); err != nil {
		if quiet && out.Len() > 0 {
			return fmt.Errorf("run upstream installer: %w\n%s", err, out.String())
		}
		return fmt.Errorf("run upstream installer: %w", err)
	}
	return nil
}

// installerInput selects the stream the upstream installers read prompts from.
// The maintained installers ask only before replacing existing local
// configuration or binaries, so --force answers yes without exposing
// configuration contents. --yes only guarantees the installer never blocks on
// a read it cannot satisfy: closed input makes it take its own default, which
// leaves existing configuration in place.
func installerInput(force, assumeYes bool) io.Reader {
	switch {
	case force:
		return strings.NewReader("y\n")
	case assumeYes:
		return strings.NewReader("")
	default:
		return os.Stdin
	}
}

func download(url, suffix string, progress ProgressFunc) (string, error) {
	client := http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download installer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download installer: %s", resp.Status)
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	reportProgress(progress, ProgressDownloading, 0, total)
	file, err := os.CreateTemp("", "elastic-docs-utils-installer-*"+suffix)
	if err != nil {
		return "", err
	}
	path := file.Name()
	reader := &progressReader{reader: resp.Body, total: total, progress: progress}
	if _, err := io.Copy(file, reader); err != nil {
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

type progressReader struct {
	reader    io.Reader
	completed int64
	total     int64
	progress  ProgressFunc
}

func (r *progressReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	if n > 0 {
		r.completed += int64(n)
		reportProgress(r.progress, ProgressDownloading, r.completed, r.total)
	}
	return n, err
}

func reportProgress(progress ProgressFunc, phase ProgressPhase, completed, total int64) {
	if progress != nil {
		progress(phase, completed, total)
	}
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
