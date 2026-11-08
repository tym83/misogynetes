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
	"testing"
)

// A character device is not a terminal: "2>/dev/null" is a script, and a
// script gets plain kubectl.
func TestOnlyARealTerminalIsATerminal(t *testing.T) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	if info, err := devNull.Stat(); err != nil || info.Mode()&os.ModeCharDevice == 0 {
		t.Fatalf("%s is expected to be a character device: %v %v", os.DevNull, info, err)
	}
	if terminal(devNull) {
		t.Errorf("%s counts as a terminal", os.DevNull)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if terminal(w) {
		t.Error("a pipe counts as a terminal")
	}

	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if terminal(f) {
		t.Error("a file counts as a terminal")
	}
}

// With stderr thrown away she is plain kubectl: errors reach where they
// were sent and nothing of hers is added to stdout.
func TestStderrToDevNullGetsPlainKubectl(t *testing.T) {
	s := newSandbox(t, "echo 'Error: boom' >&2; exit 3\n")
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	cmd := s.command([]string{"MISOGYNETES="}, "get", "pods")
	out := s.tempFile()
	cmd.Stdout, cmd.Stderr = out, devNull
	if err := cmd.Run(); err == nil || cmd.ProcessState.ExitCode() != 3 {
		t.Fatalf("exit code %d, want kubectl's 3 (%v)", cmd.ProcessState.ExitCode(), err)
	}
	if got := readAll(t, out); got != "" {
		t.Errorf("stdout %q, want nothing", got)
	}
	if _, err := os.Stat(s.statePath()); err == nil {
		t.Error("she kept a memory of a script's command")
	}
}
