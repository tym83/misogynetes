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
	"bytes"
	"errors"
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
	kubectl := os.Getenv("MISOGYNETES_KUBECTL")
	if kubectl == "" {
		kubectl = "kubectl"
	}
	if !actsUp() {
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
	c := &Cluster{Rand: r, Now: now, Day: -1, State: Load(statePath, now, r)}
	if d, err := strconv.Atoi(os.Getenv("MISOGYNETES_DAY")); err == nil {
		c.Day = d
	}
	defer Save(statePath, c.State)

	if own, rest := ownCommand(args); own != "" {
		switch own {
		case "sorry":
			reason := strings.TrimPrefix(strings.Join(rest, " "), "for ")
			say(c.Sorry(reason))
			return 0
		case "what", "whats-wrong":
			say(c.What())
			return 0
		case "flowers":
			say(c.Flowers())
			return 0
		}
	}

	plan := c.Before(args)
	say(plan.Say)
	if !plan.Run {
		return 1
	}

	var captured bytes.Buffer
	cmd := exec.Command(kubectl, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, &captured
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		_, _ = io.Copy(os.Stderr, &captured) // warnings still reach you
		return 0
	case errors.As(err, &exit):
		say(c.Failed(args, captured.String()))
		return exit.ExitCode()
	default:
		say(c.Failed(args, err.Error()))
		return 1
	}
}

// ownCommand reports her own command (sorry, what, whats-wrong, flowers),
// which counts only as the very first word. Anywhere else it is a kubectl
// argument: "--as what get pods" is kubectl's business.
func ownCommand(args []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}
	switch args[0] {
	case "sorry", "what", "whats-wrong", "flowers":
		return args[0], args[1:]
	}
	return "", nil
}

func say(lines []string) {
	for _, l := range lines {
		fmt.Fprintf(os.Stderr, "\033[35m%s\033[0m\n", l)
	}
}

// actsUp: MISOGYNETES=off makes her plain kubectl, MISOGYNETES=always makes
// her act up even into a pipe; otherwise only a person at a terminal gets it.
func actsUp() bool {
	switch os.Getenv("MISOGYNETES") {
	case "off":
		return false
	case "always":
		return true
	}
	info, err := os.Stderr.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func passthrough(kubectl string, args []string) int {
	cmd := exec.Command(kubectl, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
