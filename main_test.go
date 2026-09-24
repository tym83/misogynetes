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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if builtBin != "" {
		_ = os.RemoveAll(filepath.Dir(builtBin))
	}
	os.Exit(code)
}

// binary builds misogynectl once per test run.
func binary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "misogynectl-test-")
		if err != nil {
			buildErr = err
			return
		}
		builtBin = filepath.Join(dir, "misogynectl")
		if out, err := exec.Command("go", "build", "-o", builtBin, ".").CombinedOutput(); err != nil {
			buildErr = errors.New(err.Error() + "\n" + string(out))
		}
	})
	if buildErr != nil {
		t.Fatalf("build: %v", buildErr)
	}
	return builtBin
}

// sandbox is a temporary HOME with a fake kubectl in it.
type sandbox struct {
	t    *testing.T
	dir  string
	bin  string
	fake string
}

func newSandbox(t *testing.T, kubectlScript string) *sandbox {
	t.Helper()
	dir := t.TempDir()
	fake := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"+kubectlScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return &sandbox{t: t, dir: dir, bin: binary(t), fake: fake}
}

func (s *sandbox) command(env []string, args ...string) *exec.Cmd {
	cmd := exec.Command(s.bin, args...)
	cmd.Env = append(os.Environ(), "MISOGYNETES_KUBECTL="+s.fake, "HOME="+s.dir,
		"XDG_CACHE_HOME="+s.dir, "MISOGYNETES_DEPTH=")
	cmd.Env = append(cmd.Env, env...)
	return cmd
}

// run runs misogynectl and returns stdout, stderr and the exit code.
func (s *sandbox) run(env []string, args ...string) (string, string, int) {
	s.t.Helper()
	cmd := s.command(env, args...)
	var o, e strings.Builder
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		s.t.Fatal(err)
	}
	return o.String(), e.String(), code
}

// statePath is where misogynectl keeps its state inside the sandbox.
func (s *sandbox) statePath() string {
	s.t.Setenv("HOME", s.dir)
	s.t.Setenv("XDG_CACHE_HOME", s.dir)
	dir, err := os.UserCacheDir()
	if err != nil {
		s.t.Fatal(err)
	}
	return filepath.Join(dir, "misogynetes", "state.json")
}

// acting runs her acting up on an ordinary day, trying seeds until she lets
// kubectl run (she may refuse with exit 1; that is checked too).
func (s *sandbox) acting(extra []string, args ...string) (string, string, int) {
	s.t.Helper()
	for seed := 0; seed < 60; seed++ {
		env := append([]string{"MISOGYNETES=always", "MISOGYNETES_DAY=3",
			"MISOGYNETES_SEED=" + strconv.Itoa(seed)}, extra...)
		out, errOut, code := s.run(env, args...)
		if code == 1 && out == "" && !strings.Contains(errOut, "kubectl ran") {
			continue // she refused, and kubectl did not run
		}
		return out, errOut, code
	}
	s.t.Fatal("she never let kubectl run in 60 tries")
	return "", "", 0
}

const echoKubectl = "echo \"kubectl ran: $*\"\n"

func TestFlagValuesAreNotHerCommands(t *testing.T) {
	s := newSandbox(t, echoKubectl)
	for _, args := range [][]string{
		{"--as", "what", "get", "pods"},
		{"--user", "flowers", "get", "pods"},
		{"--cluster", "sorry", "delete", "pod", "x"},
		{"-v", "6", "get", "pods"},
	} {
		out, _, code := s.acting(nil, args...)
		if want := "kubectl ran: " + strings.Join(args, " ") + "\n"; out != want || code != 0 {
			t.Errorf("%v: stdout %q code %d, want %q and 0", args, out, code, want)
		}
	}
}
