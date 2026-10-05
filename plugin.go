/*
Copyright 2026 The Misogynetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// invokedAs is the command the user typed to reach her.
var invokedAs = commandName(os.Args[0])

// commandName is how her lines refer to the command. kubectl runs a plugin
// binary called kubectl-<name> for "kubectl <name>", so when installed that
// way (for example with krew) the hints say "kubectl misogyn what".
func commandName(argv0 string) string {
	base := strings.TrimSuffix(filepath.Base(argv0), ".exe")
	if name, ok := strings.CutPrefix(base, "kubectl-"); ok && name != "" {
		// kubectl maps an underscore in the file name to a dash.
		return "kubectl " + strings.ReplaceAll(name, "_", "-")
	}
	return "misogynectl"
}

// asInvoked rewrites the commands she suggests to match how she was called.
func asInvoked(line, name string) string {
	if name == "misogynectl" {
		return line
	}
	return strings.ReplaceAll(line, "misogynectl", name)
}
