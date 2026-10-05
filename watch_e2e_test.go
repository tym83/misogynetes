//go:build unix

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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// watchKubectl answers her looks: a crashing pod, the rest calm.
const watchKubectl = `echo "$*" >> "$0.log"
for a in "$@"; do
  case "$a" in
  pods) cat <<'T'
NAME     READY   STATUS             RESTARTS      AGE
web-1    0/1     CrashLoopBackOff   6 (40s ago)   9m
T
  exit 0 ;;
  deployments) echo '{"items":[]}'; exit 0 ;;
  nodes|events|pvc) echo "No resources found."; exit 0 ;;
  esac
done
`

// watching is a watcher process started by the test, and only by it.
type watching struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (s *sandbox) startWatcher(t *testing.T, tty string, env ...string) *watching {
	t.Helper()
	env = append([]string{"MISOGYNETES_TTY_ANY=1", "MISOGYNETES_DAY=3", "MISOGYNETES_SEED=1"}, env...)
	cmd := s.command(env, watchCommand, "--tty", tty, "--context", "ctx-a")
	cmd.Stdout, cmd.Stderr = s.tempFile(), s.tempFile()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	w := &watching{cmd: cmd, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(w.done) }()
	t.Cleanup(func() {
		select {
		case <-w.done:
		default:
			_ = cmd.Process.Kill()
			<-w.done
		}
	})
	return w
}

func (w *watching) exitsWithin(d time.Duration) bool {
	select {
	case <-w.done:
		return true
	case <-time.After(d):
		return false
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("never: %s", what)
}

func (s *sandbox) fakeTTY(t *testing.T) string {
	t.Helper()
	tty := filepath.Join(s.dir, "tty")
	if err := os.WriteFile(tty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return tty
}

func read(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func TestWatcherProcess(t *testing.T) {
	s := newSandbox(t, watchKubectl)
	tty := s.fakeTTY(t)
	w := s.startWatcher(t, tty)
	eventually(t, "she comes running", func() bool { return strings.Contains(read(tty), "pod/web-1") })
	if said := read(tty); !strings.HasPrefix(said, "\n") {
		t.Errorf("interruption does not start on a new line: %q", said)
	}
	for _, call := range strings.Split(strings.TrimSpace(read(s.fake+".log")), "\n") {
		if !strings.HasPrefix(call, "--context ctx-a --request-timeout 10s get ") || !readOnlyCall(strings.Fields(call)) {
			t.Errorf("call %q", call)
		}
	}

	// One watcher per context.
	second := s.startWatcher(t, tty)
	if !second.exitsWithin(5 * time.Second) {
		t.Error("a second watcher for the same context stayed")
	}
	if w.exitsWithin(200 * time.Millisecond) {
		t.Fatal("the first one stopped")
	}

	_, errOut, code := s.run([]string{"MISOGYNETES=always"}, "leave-me-alone")
	if code != 0 || !strings.Contains(errOut, "come-back") {
		t.Errorf("leave-me-alone: %d %q", code, errOut)
	}
	if !w.exitsWithin(5 * time.Second) {
		t.Fatal("leave-me-alone did not stop her")
	}
	calls := read(s.fake + ".log")
	again := s.startWatcher(t, tty)
	if !again.exitsWithin(5*time.Second) || read(s.fake+".log") != calls {
		t.Error("left alone, she started looking again")
	}
	s.run([]string{"MISOGYNETES=always"}, "come-back")
	back := s.startWatcher(t, tty)
	if back.exitsWithin(time.Second) {
		t.Error("come-back did not let her watch again")
	}
}

func TestWatcherStopsOnOffTTYAndLifetime(t *testing.T) {
	s := newSandbox(t, watchKubectl)
	tty := s.fakeTTY(t)

	w := s.startWatcher(t, tty)
	eventually(t, "she comes running", func() bool { return read(tty) != "" })
	state := read(s.statePath())
	out, errOut, code := s.run([]string{"MISOGYNETES=off"}, "get", "nodes")
	if out != "No resources found.\n" || errOut != "" || code != 0 {
		t.Errorf("off: stdout %q stderr %q code %d", out, errOut, code)
	}
	if !w.exitsWithin(5 * time.Second) {
		t.Error("MISOGYNETES=off did not stop her")
	}
	if read(s.statePath()) != state {
		t.Error("MISOGYNETES=off touched the state")
	}

	w = s.startWatcher(t, tty)
	eventually(t, "a second look", func() bool { return strings.Count(read(s.fake+".log"), "get pods") >= 2 })
	if err := os.Remove(tty); err != nil {
		t.Fatal(err)
	}
	if !w.exitsWithin(5 * time.Second) {
		t.Error("the terminal went away and she stayed")
	}
	if _, err := os.Stat(tty); err == nil {
		t.Error("she created the terminal")
	}

	tty = s.fakeTTY(t)
	w = s.startWatcher(t, tty, "MISOGYNETES_WATCH_LIFETIME=1s")
	if !w.exitsWithin(5 * time.Second) {
		t.Error("she outlived her lifetime")
	}
}

func TestScriptsNeverStartAWatcher(t *testing.T) {
	s := newSandbox(t, watchKubectl)
	for _, env := range [][]string{{"MISOGYNETES="}, {"MISOGYNETES=always", "MISOGYNETES_DAY=3", "MISOGYNETES_SEED=0"}, {"MISOGYNETES=off"}} {
		s.run(env, "get", "pods")
	}
	time.Sleep(300 * time.Millisecond)
	pids, _ := filepath.Glob(filepath.Join(filepath.Dir(s.statePath()), "watch-*"))
	if len(pids) != 0 {
		t.Errorf("a watcher was started: %v", pids)
	}
	if n := strings.Count(read(s.fake+".log"), "\n"); n != 3 {
		t.Errorf("%d kubectl calls for 3 commands", n)
	}
}
