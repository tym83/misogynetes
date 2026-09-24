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
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// holdFor is how long she may keep kubectl's stderr to herself. After that
// it goes out as it comes: a login prompt or a warning must not wait.
const holdFor = 3 * time.Second

// maxHeld is how much stderr she keeps back: the last 8KB.
const maxHeld = 8 << 10

// quickVerbs are the short, non-interactive commands whose error she may
// hide. Everything else (exec, attach, run, debug, port-forward, proxy,
// edit, logs, cp, auth, plugins, ...) gets its stderr straight through.
var quickVerbs = map[string]bool{
	"get": true, "describe": true, "apply": true, "create": true, "delete": true,
	"scale": true, "label": true, "annotate": true, "patch": true, "rollout": true,
	"set": true, "top": true, "version": true, "api-resources": true, "explain": true,
	"config": true, "cordon": true, "uncordon": true, "taint": true,
}

// quickRollouts are the rollout subcommands that count as quick.
var quickRollouts = map[string]bool{"status": true, "restart": true, "undo": true}

// liveFlags make any command interactive or long-running.
var liveFlags = map[string]bool{
	"-i": true, "-t": true, "-it": true, "-ti": true, "--stdin": true, "--tty": true,
	"--interactive": true, "-w": true, "--watch": true, "--watch-only": true,
	"--edit": true, "--follow": true,
}

// holdsStderr reports whether she may keep kubectl's stderr back for this
// command: only for a short command that asks nothing and watches nothing.
func holdsStderr(args []string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}
		name, _, _ := strings.Cut(a, "=")
		if liveFlags[name] {
			return false
		}
	}
	words := Words(args)
	if len(words) == 0 || !quickVerbs[words[0]] {
		return false
	}
	if words[0] == "rollout" {
		return len(words) > 1 && quickRollouts[words[1]]
	}
	return true
}

// holdBack keeps the tail of kubectl's stderr until it is released; from
// then on everything goes straight to the terminal.
type holdBack struct {
	mu      sync.Mutex
	held    []byte
	trimmed bool
	out     io.Writer // nil while holding
}

func (h *holdBack) Write(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.out != nil {
		return h.out.Write(p)
	}
	h.held = append(h.held, p...)
	if over := len(h.held) - maxHeld; over > 0 {
		h.held = append(h.held[:0], h.held[over:]...)
		h.trimmed = true
	}
	return len(p), nil
}

// release lets out what was held and passes everything after it through.
func (h *holdBack) release(out io.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.out != nil {
		return
	}
	if h.trimmed {
		_, _ = io.WriteString(out, "[... earlier stderr trimmed ...]\n")
	}
	_, _ = out.Write(h.held)
	h.held, h.out = nil, out
}

// kept returns what is still held back, and false if it was released.
func (h *holdBack) kept() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return string(h.held), h.out == nil
}

// holdTime is holdFor, or shorter when MISOGYNETES_HOLD says so (for tests;
// it can never be made longer).
func holdTime() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("MISOGYNETES_HOLD")); err == nil && d >= 0 && d < holdFor {
		return d
	}
	return holdFor
}
