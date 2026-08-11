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

// Package ui renders Elastic Docs Utils command-line output.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// ColorMode controls ANSI color output.
type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

// Renderer renders human-readable command output. JSON-producing callers do
// not use this type.
type Renderer struct {
	out         io.Writer
	err         io.Writer
	color       bool
	interactive bool
	verbose     bool
}

// Progress is a transient activity indicator for work whose duration cannot
// be measured precisely, such as downloads and external installers.
type Progress struct {
	r           *Renderer
	interactive bool
	stop        chan struct{}
	done        chan struct{}
	stopOnce    sync.Once
	labelMu     sync.RWMutex
	label       string
}

// SetVerbose enables diagnostic file-change output for mutating commands.
func (r *Renderer) SetVerbose(enabled bool) { r.verbose = enabled }

// New creates a renderer. ColorAuto honors NO_COLOR, TERM=dumb, and whether
// stdout is an interactive terminal.
func New(mode ColorMode, out, errOut io.Writer) *Renderer {
	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}
	return &Renderer{
		out:         out,
		err:         errOut,
		color:       colorEnabled(mode, out),
		interactive: interactiveOutput(out),
	}
}

func interactiveOutput(out io.Writer) bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func colorEnabled(mode ColorMode, out io.Writer) bool {
	if mode == ColorAlways {
		return true
	}
	if mode == ColorNever || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

const wordmark = "    ________           __  _         ____                     __  ____  _ __    \n" +
	"   / ____/ /___ ______/ /_(_)____   / __ \\____  __________   / / / / /_(_) /____\n" +
	"  / __/ / / __ ` / ___/ __/ / ___/ / / / / __ \\/ ___/ ___/  / / / / __/ / / ___/\n" +
	" / /___/ / /_/ (__  ) /_/ / /__   / /_/ / /_/ / /__(__  )  / /_/ / /_/ / (__  ) \n" +
	"/_____/_/\\__,_/____/\\__/_/\\___/  /_____/\\____/\\___/____/   \\____/\\__/_/_/____/"

const compactWordmark = `+--------------------+
| ELASTIC DOCS UTILS |
+--------------------+`

const wideHeaderColumns = 90

func (r *Renderer) paint(code, text string) string {
	if !r.color {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

// Header renders the large interactive command banner.
func (r *Renderer) Header(version string) {
	wordmark := wordmarkForWidth(terminalWidth(r.out))
	fmt.Fprintln(r.out, r.paint("36;1", wordmark))
	fmt.Fprintf(r.out, "%s\n\n", r.paint("35", "                         Elastic Docs Utils | v"+version))
}

func wordmarkForWidth(width int) string {
	if width > 0 && width < wideHeaderColumns {
		return compactWordmark
	}
	return wordmark
}

func terminalWidth(out io.Writer) int {
	file, ok := out.(*os.File)
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil {
		return 0
	}
	return width
}

func (r *Renderer) Section(format string, args ...any) {
	fmt.Fprintf(r.out, "%s\n", r.paint("36;1", fmt.Sprintf(format, args...)))
}

func (r *Renderer) Info(format string, args ...any) {
	fmt.Fprintf(r.out, "%s %s\n", r.paint("36;1", "[INFO]"), fmt.Sprintf(format, args...))
}

// StartProgress shows an animated spinner on terminals until Stop is called.
// Redirected output receives one durable start line instead, which makes long
// operations visible in CI logs without emitting cursor-control sequences.
func (r *Renderer) StartProgress(format string, args ...any) *Progress {
	label := fmt.Sprintf(format, args...)
	if !r.interactive {
		r.Info("%s...", strings.TrimRight(label, "."))
		return &Progress{r: r, label: label}
	}

	p := &Progress{r: r, interactive: true, stop: make(chan struct{}), done: make(chan struct{}), label: label}
	frames := []string{"|", "/", "-", "\\"}
	p.draw(frames[0])
	go func() {
		defer close(p.done)
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		frame := 1
		for {
			select {
			case <-ticker.C:
				p.draw(frames[frame])
				frame = (frame + 1) % len(frames)
			case <-p.stop:
				return
			}
		}
	}()
	return p
}

func (p *Progress) draw(frame string) {
	p.labelMu.RLock()
	defer p.labelMu.RUnlock()
	fmt.Fprintf(p.r.out, "\r\033[2K%s %s", p.r.paint("36;1", "["+frame+"]"), p.label)
}

// Update changes the current progress detail. Interactive output updates the
// spinner in place; redirected output receives a durable line for each step.
func (p *Progress) Update(format string, args ...any) {
	if p == nil {
		return
	}
	label := fmt.Sprintf(format, args...)
	p.labelMu.Lock()
	if p.label == label {
		p.labelMu.Unlock()
		return
	}
	p.label = label
	p.labelMu.Unlock()
	if !p.interactive {
		p.r.Info("%s", label)
		return
	}
}

// Stop removes an interactive progress line. It is safe to call more than
// once and is a no-op for redirected output.
func (p *Progress) Stop() {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		if p.stop == nil {
			return
		}
		close(p.stop)
		<-p.done
		fmt.Fprint(p.r.out, "\r\033[2K")
	})
}

// Verbose describes an owned file or configuration location changed by a
// command. It never prints secrets or configuration contents.
func (r *Renderer) Verbose(format string, args ...any) {
	if !r.verbose {
		return
	}
	fmt.Fprintf(r.out, "%s %s\n", r.paint("90", "[VERBOSE]"), fmt.Sprintf(format, args...))
}

func (r *Renderer) Success(format string, args ...any) {
	fmt.Fprintf(r.out, "%s %s\n", r.paint("32;1", "[OK]"), fmt.Sprintf(format, args...))
}

func (r *Renderer) Warn(format string, args ...any) {
	fmt.Fprintf(r.out, "%s %s\n", r.paint("33;1", "[WARN]"), fmt.Sprintf(format, args...))
}

func (r *Renderer) Error(format string, args ...any) {
	fmt.Fprintf(r.err, "%s %s\n", r.paint("31;1", "[ERROR]"), fmt.Sprintf(format, args...))
}

func (r *Renderer) DryRun() {
	fmt.Fprintln(r.out, r.paint("33;1", "[DRY RUN] No changes will be made."))
}

// Table renders compact aligned rows. Each row must contain the same number
// of columns; narrow or non-terminal output remains readable plain text.
func (r *Renderer) Table(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, value := range row {
			if i < len(widths) && len(value) > widths[i] {
				widths[i] = len(value)
			}
		}
	}
	write := func(values []string, heading bool) {
		parts := make([]string, 0, len(values))
		for i, value := range values {
			if i >= len(widths) {
				break
			}
			parts = append(parts, fmt.Sprintf("%-*s", widths[i], value))
		}
		line := strings.TrimRight(strings.Join(parts, "  "), " ")
		if heading {
			line = r.paint("36;1", line)
		}
		fmt.Fprintln(r.out, line)
	}
	write(headers, true)
	for _, row := range rows {
		write(row, false)
	}
}
