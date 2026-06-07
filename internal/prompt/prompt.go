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

// Package prompt provides simple line-based interactive prompts with flag fallbacks.
package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var reader = bufio.NewReader(os.Stdin)

// YesOrNo asks a yes/no question and returns true for yes.
// defaultYes controls what Enter (no input) means.
// In non-interactive mode (--yes flag or non-TTY) returns defaultYes.
func YesOrNo(question string, defaultYes bool, yes bool) bool {
	if yes || !isInteractive() {
		return defaultYes
	}
	hint := "Y/n"
	if !defaultYes {
		hint = "y/N"
	}
	fmt.Printf("%s [%s]: ", question, hint)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}

// String asks for a string value with an optional default.
// In non-interactive mode returns the defaultValue.
func String(question, defaultValue string, yes bool) string {
	if yes || !isInteractive() {
		return defaultValue
	}
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", question, defaultValue)
	} else {
		fmt.Printf("%s: ", question)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue
	}
	return line
}

// Select asks the user to pick from a numbered list of options.
// Returns the chosen index (0-based). In non-interactive mode returns defaultIdx.
func Select(question string, options []string, defaultIdx int, yes bool) int {
	if yes || !isInteractive() {
		return defaultIdx
	}
	fmt.Printf("%s\n", question)
	for i, opt := range options {
		marker := "  "
		if i == defaultIdx {
			marker = "* "
		}
		fmt.Printf("%s%d) %s\n", marker, i+1, opt)
	}
	fmt.Printf("Choice [%d]: ", defaultIdx+1)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultIdx
	}
	var idx int
	if _, err := fmt.Sscanf(line, "%d", &idx); err == nil && idx >= 1 && idx <= len(options) {
		return idx - 1
	}
	return defaultIdx
}

// Info prints an informational message (cyan ℹ).
func Info(format string, args ...any) {
	fmt.Printf("\033[0;36m\033[1mℹ\033[0m "+format+"\n", args...)
}

// OK prints a success message (green ✓).
func OK(format string, args ...any) {
	fmt.Printf("\033[0;32m\033[1m✓\033[0m "+format+"\n", args...)
}

// Warn prints a warning message (yellow ⚠).
func Warn(format string, args ...any) {
	fmt.Printf("\033[0;33m\033[1m⚠\033[0m "+format+"\n", args...)
}

// Err prints an error message (red ✗) to stderr.
func Err(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[0;31m\033[1m✗\033[0m "+format+"\n", args...)
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
