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

// Command misogynectl is kubectl that behaves exactly the way misogynists
// think women behave. It is a punishment for them.
//
// It never runs anything other than the command it was given: it either
// runs it or, in a mood, refuses. Exit codes are honest. It only acts up for
// a person at a terminal; in pipes and scripts it is plain kubectl.
package main

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if !actsUp() {
		kubectl, err := resolveKubectl()
		if err != nil {
			return wrapperError(err)
		}
		return passthrough(kubectl, args)
	}

	seed := time.Now().UnixNano()
	if s, err := strconv.ParseInt(os.Getenv("MISOGYNETES_SEED"), 10, 64); err == nil {
		seed = s
	}
	r := rand.New(rand.NewSource(seed))
	statePath := ""
	if dir, err := os.UserCacheDir(); err == nil {
		statePath = filepath.Join(dir, "misogynetes", "state.json")
	}
	now := time.Now()
	c := &Cluster{Rand: r, Now: now, Day: -1}
	if d, err := strconv.Atoi(os.Getenv("MISOGYNETES_DAY")); err == nil {
		c.Day = d
	}
	// update runs one step of her reasoning on the freshest state, under a
	// lock. The lock is never held while kubectl runs.
	update := func(step func()) {
		Update(statePath, now, r, func(s *State) {
			c.State = s
			step()
		})
	}

	if own, rest := ownCommand(args); own == "about" {
		fmt.Println(about)
		return 0
	} else if own != "" {
		var lines []string
		update(func() {
			switch own {
			case "sorry":
				if len(rest) > 0 && rest[0] == "for" {
					rest = rest[1:]
				}
				lines = c.Sorry(strings.Join(rest, " "))
			case "what", "whats-wrong":
				lines = c.What()
			case "flowers":
				lines = c.Flowers()
			case "aga":
				lines = c.Aga()
			}
		})
		say(lines)
		return 0
	}

	kubectl, err := resolveKubectl()
	if err != nil {
		return wrapperError(err)
	}
	var plan Plan
	update(func() { plan = c.Before(args) })
	say(plan.Say)
	if !plan.Run {
		if plan.Code != 0 {
			return plan.Code
		}
		return 1
	}

	cmd := kubectlCommand(kubectl, args)
	var output *seen
	if listens(args) {
		// She reads along: every byte still goes to the terminal as it
		// comes, and she only reads the output of the command you typed.
		output = &seen{}
		cmd.Stdout = io.MultiWriter(os.Stdout, output)
	}
	code, err := runKubectl(cmd, args, func(stderr string) {
		var lines []string
		update(func() { lines = c.Failed(args, stderr) })
		say(lines)
	})
	if err != nil {
		return wrapperError(err)
	}
	if output != nil && code == 0 {
		var lines []string
		update(func() { lines = c.Observe(args, output.String()) })
		say(lines)
	}
	return code
}

// runKubectl runs kubectl and returns its exit code. For a quick command it
// keeps a failure's stderr back and hands it to failed instead.
func runKubectl(cmd *exec.Cmd, args []string, failed func(stderr string)) (int, error) {
	if !holdsStderr(args) {
		// Interactive or long-running: its stderr is part of the
		// conversation (prompts, watch warnings), so nothing is hidden.
		return execute(cmd)
	}

	held := &holdBack{}
	cmd.Stderr = held
	timer := time.AfterFunc(holdTime(), func() { held.release(os.Stderr) })
	code, err := execute(cmd)
	timer.Stop()
	stderr, hidden := held.kept()
	if err != nil || code == 0 || !hidden {
		held.release(os.Stderr) // warnings, and anything already on its way
		return code, err
	}
	failed(stderr)
	fmt.Fprintf(os.Stderr, hiddenHint+"\n", code)
	return code, nil
}

// ownCommand reports her own command (sorry, what, whats-wrong, flowers,
// aga and its synonyms, about), which counts only as the very first word.
// Anywhere else it is a kubectl argument: "--as what get pods" is kubectl's
// business.
func ownCommand(args []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}
	switch args[0] {
	case "sorry", "what", "whats-wrong", "flowers", "about":
		return args[0], args[1:]
	case "aga", "ага", "угу", "uh-huh", "mhm", "aha", "yeah":
		return "aga", args[1:]
	}
	return "", nil
}

func say(lines []string) {
	for _, l := range lines {
		fmt.Fprintf(os.Stderr, "\033[35m%s\033[0m\n", l)
	}
}

// actsUp: MISOGYNETES=off makes her plain kubectl, MISOGYNETES=always makes
// her act up even into a pipe; otherwise only a person at a terminal gets it,
// with both stdout and stderr on it: "misogynectl get pods | grep x" is a
// script, and a script gets plain kubectl.
func actsUp() bool {
	switch os.Getenv("MISOGYNETES") {
	case "off":
		return false
	case "always":
		return true
	}
	return terminal(os.Stdout) && terminal(os.Stderr)
}

func terminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func passthrough(kubectl string, args []string) int {
	code, err := execute(kubectlCommand(kubectl, args))
	if err != nil {
		return wrapperError(err)
	}
	return code
}
