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
	"errors"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// watchCommand is the hidden first word that runs the watcher itself.
const watchCommand = "__watch"

// findTTY is the path of the terminal on stderr, found by its device
// number, or "" when there is none.
func findTTY() string {
	info, err := os.Stderr.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return ""
	}
	want, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	for _, pattern := range []string{"/dev/pts/*", "/dev/ttys*", "/dev/tty[0-9]*"} {
		paths, _ := filepath.Glob(pattern)
		for _, p := range paths {
			if fi, err := os.Stat(p); err == nil {
				if st, ok := fi.Sys().(*syscall.Stat_t); ok && fi.Mode()&os.ModeCharDevice != 0 && uint64(st.Rdev) == uint64(want.Rdev) {
					return p
				}
			}
		}
	}
	return ""
}

// ttyAlive reports a terminal that still exists and still belongs to the
// user. anyFile lets a plain file stand in for it (tests only).
func ttyAlive(path string, anyFile bool) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Getuid() {
		return false
	}
	return anyFile || fi.Mode()&os.ModeCharDevice != 0
}

// writeTTY interrupts you. The terminal is never created, only written to.
func writeTTY(path string, anyFile bool, lines []string) error {
	if !ttyAlive(path, anyFile) {
		return errors.New("terminal gone")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(ttyLines(lines))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// tryLock takes an exclusive lock without waiting. It returns the release
// function, or nil if someone else holds it.
func tryLock(path string) func() {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		_ = f.Close()
		return nil
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
}

// ensureWatcher starts her watcher for this command's context, unless one
// is already watching it. Only called for a person at a terminal.
func ensureWatcher(args []string, dir string) {
	if dir == "" || os.Getenv("MISOGYNETES_WATCH") == "off" {
		return
	}
	context := currentContext(args)
	tty := findTTY()
	if context == "" || tty == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	key := watchKey(dir, context)
	release := tryLock(key + ".lock")
	if release == nil {
		return // she is already watching
	}
	release()
	self, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(self, watchCommand, "--tty", tty, "--context", context)
	cmd.Env = os.Environ()
	if p := flagValue(args, "--kubeconfig"); p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			cmd.Env = append(cmd.Env, "KUBECONFIG="+abs)
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if cmd.Start() == nil {
		_ = cmd.Process.Release()
	}
}

// stopWatchers stops every watcher of this user: only a process that holds
// a watcher's lock, by the pid it wrote under that lock.
func stopWatchers(dir string) {
	if dir == "" {
		return
	}
	pids, _ := filepath.Glob(filepath.Join(dir, "watch-*.pid"))
	for _, pidFile := range pids {
		key := strings.TrimSuffix(pidFile, ".pid")
		if release := tryLock(key + ".lock"); release != nil {
			release() // nobody is watching
			continue
		}
		b, err := os.ReadFile(pidFile)
		if err != nil {
			continue
		}
		if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 1 {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
}

// watchMain is the watcher process: one per user and context, detached,
// at low priority, until it is told to stop or has nothing left to watch.
func watchMain(args []string) int {
	tty, context := flagValue(args, "--tty"), flagValue(args, "--context")
	statePath := statePathFor()
	if tty == "" || context == "" || statePath == "" {
		return 2
	}
	dir := filepath.Dir(statePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 1
	}
	key := watchKey(dir, context)
	release := tryLock(key + ".lock")
	if release == nil {
		return 0 // another one is already watching this context
	}
	defer release()
	_ = os.WriteFile(key+".pid", []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
	defer os.Remove(key + ".pid")
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, 0, 10)

	kubectl, err := resolveKubectl()
	if err != nil {
		return 1
	}
	seed := time.Now().UnixNano()
	if s, err := strconv.ParseInt(os.Getenv("MISOGYNETES_SEED"), 10, 64); err == nil {
		seed = s
	}
	day := -1
	if d, err := strconv.Atoi(os.Getenv("MISOGYNETES_DAY")); err == nil {
		day = d
	}
	anyFile := os.Getenv("MISOGYNETES_TTY_ANY") == "1"
	w := &Watcher{
		Context: context, All: os.Getenv("MISOGYNETES_WATCH_ALL") == "1",
		StatePath: statePath, Rand: rand.New(rand.NewSource(seed)), Day: day,
		Poll:     envDuration("MISOGYNETES_POLL", defaultPoll, minPoll),
		Nag:      envDuration("MISOGYNETES_NAG", defaultNag, minNag),
		Lifetime: lifetime(), Started: time.Now(), Now: time.Now,
		Run:   kubectlRunner(kubectl),
		Alive: func() bool { return ttyAlive(tty, anyFile) },
		Write: func(lines []string) error { return writeTTY(tty, anyFile, lines) },
	}
	if os.Getenv("MISOGYNETES_NOTIFY") == "1" && runtime.GOOS == "darwin" {
		w.Notify = notify
	}

	// Told to stop, she stops at once, even in the middle of a look.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		<-sigs
		_ = os.Remove(key + ".pid")
		os.Exit(0)
	}()
	for w.Tick() {
		time.Sleep(250 * time.Millisecond)
	}
	return 0
}
