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
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
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
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho >> \"$0.ran\"\n"+kubectlScript), 0o755); err != nil {
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
	// Files, not pipes: a background child of the fake kubectl must not keep
	// the test waiting.
	o, e := s.tempFile(), s.tempFile()
	cmd.Stdout, cmd.Stderr = o, e
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		s.t.Fatal(err)
	}
	return readAll(s.t, o), readAll(s.t, e), code
}

func (s *sandbox) tempFile() *os.File {
	f, err := os.CreateTemp(s.dir, "out-*")
	if err != nil {
		s.t.Fatal(err)
	}
	s.t.Cleanup(func() { _ = f.Close() })
	return f
}

func readAll(t *testing.T, f *os.File) string {
	t.Helper()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// kubectlRan reports whether the fake kubectl ran since the last check.
func (s *sandbox) kubectlRan() bool {
	_, err := os.Stat(s.fake + ".ran")
	return err == nil
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
		_ = os.Remove(s.fake + ".ran")
		out, errOut, code := s.run(env, args...)
		if !s.kubectlRan() {
			if code != 1 || !strings.Contains(errOut, "\033[35m") {
				s.t.Fatalf("refused without exit 1 and a word: code %d, stderr %q", code, errOut)
			}
			continue
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

func TestTokenNeverReachesStateOrWhat(t *testing.T) {
	const jwt = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhZG1pbiJ9.c2lnbmF0dXJl"
	s := newSandbox(t, "echo \"error: the server rejected token $1\" >&2\nexit 1\n")
	if _, errOut, code := s.acting(nil, "--token="+jwt, "get", "pods"); code != 1 || strings.Contains(errOut, "rejected") {
		t.Fatalf("stderr %q code %d", errOut, code)
	}
	state, err := os.ReadFile(s.statePath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), "eyJ") {
		t.Errorf("token in state.json: %s", state)
	}
	var said strings.Builder
	for i := 0; i < 3; i++ {
		_, errOut, _ := s.run([]string{"MISOGYNETES=always"}, "what")
		said.WriteString(errOut)
	}
	if strings.Contains(said.String(), "eyJ") || !strings.Contains(said.String(), "you ran: get pods") {
		t.Errorf("what said: %s", said.String())
	}
}

func TestParallelRunsLoseNothing(t *testing.T) {
	s := newSandbox(t, echoKubectl)
	path := s.statePath()
	installed := t0.Add(-100 * 24 * time.Hour)
	at := t0
	writeState(t, path, &State{Installed: installed, Offset: 7, Mood: 1, Grudge: "delete pod x",
		GrudgeAt: at, LastError: "boom", BannerShown: true})

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.command([]string{"MISOGYNETES=always"}, "what").Run(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got State
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("state.json broken: %v\n%s", err, b)
	}
	if got.Asked != n || !got.Installed.Equal(installed) || got.Offset != 7 || got.Grudge != "delete pod x" {
		t.Errorf("lost updates: asked %d of %d, %+v", got.Asked, n, got)
	}
}

func TestUnreadableStateIsKeptAndSheIsCalm(t *testing.T) {
	s := newSandbox(t, echoKubectl)
	path := s.statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	const broken = `{"installed": "2026-01-01T00:00:00Z", "mood": 3, "grudge": "del`
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	for seed := 0; seed < 10; seed++ {
		_, errOut, _ := s.run([]string{"MISOGYNETES=always", "MISOGYNETES_SEED=" + strconv.Itoa(seed)}, "get", "pods")
		if strings.Contains(errOut, doWhatYouWant) || strings.Contains(errOut, pmsNotice) {
			t.Errorf("not calm with an unreadable state: %q", errOut)
		}
	}
	if b, _ := os.ReadFile(path); string(b) != broken {
		t.Errorf("unreadable state overwritten: %s", b)
	}
}

func writeState(t *testing.T, path string, st *State) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSignalledKubectlGivesShellExitCode(t *testing.T) {
	s := newSandbox(t, "kill -TERM $$\n")
	if _, _, code := s.run([]string{"MISOGYNETES=off"}, "get", "pods"); code != 128+15 {
		t.Errorf("plain: exit %d, want 143", code)
	}
	if _, _, code := s.acting(nil, "get", "pods"); code != 128+15 {
		t.Errorf("acting up: exit %d, want 143", code)
	}
}

func TestSignalsReachKubectlAndSheStillRemembers(t *testing.T) {
	script := `trap 'kill $pid 2>/dev/null; echo "Error: interrupted" >&2; exit 42' INT TERM HUP
echo ready
sleep 30 & pid=$!
wait $pid
`
	for _, tc := range []struct {
		sig  syscall.Signal
		mode string
	}{{syscall.SIGTERM, "off"}, {syscall.SIGINT, "off"}, {syscall.SIGHUP, "off"}, {syscall.SIGINT, "always"}} {
		s := newSandbox(t, script)
		for seed := 0; ; seed++ {
			if seed == 60 {
				t.Fatal("she never let kubectl run")
			}
			_ = os.Remove(s.fake + ".ran")
			cmd := s.command([]string{"MISOGYNETES=" + tc.mode, "MISOGYNETES_DAY=3", "MISOGYNETES_SEED=" + strconv.Itoa(seed)}, "get", "pods")
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stderr = s.tempFile()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			line, _ := bufio.NewReader(stdout).ReadString('\n')
			if line != "ready\n" {
				_ = cmd.Wait()
				continue // she refused
			}
			_ = cmd.Process.Signal(tc.sig)
			err = cmd.Wait()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != 42 {
				t.Errorf("%s %v: %v, want kubectl's exit 42", tc.mode, tc.sig, err)
			}
			break
		}
		if tc.mode != "always" {
			continue
		}
		b, err := os.ReadFile(s.statePath())
		if err != nil || !strings.Contains(string(b), "interrupted") {
			t.Errorf("state not saved after %v: %v %s", tc.sig, err, b)
		}
	}
}

func TestBackgroundChildDoesNotHangHer(t *testing.T) {
	s := newSandbox(t, "(sleep 20) &\necho out\n")
	start := time.Now()
	out, _, code := s.acting(nil, "get", "pods")
	if out != "out\n" || code != 0 || time.Since(start) > 10*time.Second {
		t.Errorf("stdout %q code %d after %v", out, code, time.Since(start))
	}
}

func TestWrapperErrorsAreNotFine(t *testing.T) {
	s := newSandbox(t, echoKubectl)
	_, errOut, code := s.run([]string{"MISOGYNETES=always", "MISOGYNETES_DAY=3", "MISOGYNETES_KUBECTL=" + filepath.Join(s.dir, "nope")}, "get", "pods")
	if code != 1 || !strings.Contains(errOut, "misogynectl:") || strings.Contains(errOut, fineAfterError) {
		t.Errorf("kubectl missing: code %d stderr %q", code, errOut)
	}
}

func TestNoLoopThroughItself(t *testing.T) {
	s := newSandbox(t, echoKubectl)
	self := filepath.Join(s.dir, "self", "kubectl")
	if err := os.MkdirAll(filepath.Dir(self), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(s.bin, self); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"off", "always"} {
		_, errOut, code := s.run([]string{"MISOGYNETES=" + mode, "MISOGYNETES_KUBECTL=" + self}, "get", "pods")
		if code != 1 || !strings.Contains(errOut, "misogynectl itself") {
			t.Errorf("%s: kubectl is misogynectl: code %d stderr %q", mode, code, errOut)
		}
	}

	loop := filepath.Join(s.dir, "loop", "kubectl")
	if err := os.MkdirAll(filepath.Dir(loop), 0o700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nexec " + s.bin + " \"$@\"\n"
	if err := os.WriteFile(loop, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := s.run([]string{"MISOGYNETES=off", "MISOGYNETES_KUBECTL=" + loop}, "get", "pods")
	if code != 1 || !strings.Contains(errOut, depthVar) {
		t.Errorf("loop through a script: code %d stderr %q", code, errOut)
	}
}
