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
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// waitDelay bounds how long she waits for kubectl's output to close after
// kubectl itself has exited (a background child may hold it open).
const waitDelay = 2 * time.Second

// depthVar counts how deep misogynectl is running inside itself, in case
// "kubectl" on the PATH is a script that calls misogynectl again.
const (
	depthVar = "MISOGYNETES_DEPTH"
	maxDepth = 2
)

// resolveKubectl finds the real kubectl and refuses one that is misogynectl
// itself, which would only call itself forever.
func resolveKubectl() (string, error) {
	name := os.Getenv("MISOGYNETES_KUBECTL")
	if name == "" {
		name = "kubectl"
	}
	if d, _ := strconv.Atoi(os.Getenv(depthVar)); d >= maxDepth {
		return "", fmt.Errorf("%q runs misogynectl again (%s=%d); set MISOGYNETES_KUBECTL to the real kubectl", name, depthVar, d)
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	if self, err := os.Executable(); err == nil && sameFile(self, path) {
		return "", fmt.Errorf("%q is misogynectl itself; set MISOGYNETES_KUBECTL to the real kubectl", path)
	}
	return path, nil
}

func sameFile(a, b string) bool {
	ai, err1 := os.Stat(a)
	bi, err2 := os.Stat(b)
	if err1 == nil && err2 == nil {
		return os.SameFile(ai, bi)
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// kubectlCommand prepares kubectl with the given arguments, passing its
// own depth on so a loop through the PATH is caught.
func kubectlCommand(kubectl string, args []string) *exec.Cmd {
	cmd := exec.Command(kubectl, args...)
	d, _ := strconv.Atoi(os.Getenv(depthVar))
	cmd.Env = append(os.Environ(), depthVar+"="+strconv.Itoa(d+1))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.WaitDelay = waitDelay
	return cmd
}

// execute runs kubectl to the end and returns its exit code, the way a shell
// reports it: 128+n when a signal killed it. SIGINT, SIGTERM and SIGHUP sent
// to misogynectl are passed on to kubectl, and she waits for it to finish
// rather than dying first, so what she has to write down still gets
// written. The error is set only when kubectl could not be run at all.
func execute(cmd *exec.Cmd) (int, error) {
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	if err := cmd.Start(); err != nil {
		return 1, err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case s := <-sigs:
				_ = cmd.Process.Signal(s)
			case <-done:
				return
			}
		}
	}()
	err := cmd.Wait()
	if cmd.ProcessState == nil {
		return 1, err
	}
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal()), nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	return cmd.ProcessState.ExitCode(), nil // also after exec.ErrWaitDelay
}

// wrapperError reports a problem of misogynectl itself, as it is: never
// hidden behind "Everything's fine".
func wrapperError(err error) int {
	fmt.Fprintf(os.Stderr, "misogynectl: %v\n", err)
	return 1
}
