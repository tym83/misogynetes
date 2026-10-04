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
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// She comes running. The first command typed at a terminal starts her
// watcher in the background: it looks at the cluster on its own, read-only,
// pinned to one context, and when it sees trouble it interrupts you on the
// terminal you started it from, again and again, until you say "aga".

const (
	defaultPoll = 30 * time.Second
	minPoll     = 5 * time.Second
	defaultNag  = 3 * time.Minute
	minNag      = time.Minute
	maxLifetime = 12 * time.Hour
	maxBackoff  = 5 * time.Minute
	callTimeout = 20 * time.Second
	// checkEvery is how often she reads her state between looks.
	checkEvery = 15 * time.Second
	// maxCallOutput is how much of one call's output she reads.
	maxCallOutput = 8 << 20
)

// Watcher is her looking at the cluster on her own.
type Watcher struct {
	Context   string
	All       bool // every namespace, not only the context's own
	StatePath string
	Rand      *rand.Rand
	Day       int
	Poll      time.Duration
	Nag       time.Duration
	Lifetime  time.Duration
	Started   time.Time

	Now    func() time.Time
	Run    func(args []string) (stdout, stderr string, code int) // read-only kubectl
	Alive  func() bool                                           // the terminal is still there and hers
	Write  func(lines []string) error                            // to that terminal
	Notify func(lines []string)

	nextPoll, nextCheck time.Time
	backoff             time.Duration
}

// watchedResources are the only things she looks at, and only with "get".
// Never secrets, never configmaps.
var watchedResources = map[string]bool{"pods": true, "nodes": true, "events": true, "pvc": true, "deployments": true}

// readOnlyCall is the guard on every call she makes on her own: "get" of a
// watched resource, pinned to an explicit context. Anything else is never
// run.
func readOnlyCall(args []string) bool {
	if len(args) < 2 || args[0] != "--context" || args[1] == "" {
		return false
	}
	words := Words(args)
	return len(words) == 2 && words[0] == "get" && watchedResources[words[1]]
}

// Tick is one step of her watching. It returns false when she stops.
func (w *Watcher) Tick() bool {
	now := w.Now()
	if now.Sub(w.Started) >= w.Lifetime || os.Getenv("MISOGYNETES") == "off" || !w.Alive() {
		return false
	}
	var snap *Snapshot
	if !now.Before(w.nextPoll) {
		if st, ok := Load(w.StatePath, now, w.Rand); ok && st.LeftAlone {
			return false // not one more look
		}
		s := w.look()
		snap = &s
		if s.APIError != "" {
			w.backoff = min(max(2*w.backoff, w.Poll), maxBackoff)
		} else {
			w.backoff = 0
		}
		w.nextPoll = now.Add(w.Poll + w.backoff)
	}
	if snap == nil && now.Before(w.nextCheck) {
		return true
	}
	w.nextCheck = now.Add(checkEvery)

	var lines []string
	stop := false
	Update(w.StatePath, now, w.Rand, func(st *State) {
		if st.LeftAlone {
			stop = true
			return
		}
		c := &Cluster{Rand: w.Rand, Now: now, Day: w.Day, State: st}
		if snap != nil {
			lines = append(lines, c.observe(snap.sightings())...)
			lines = append(lines, c.updateTalk(w.Context, Classify(*snap))...)
			if len(lines) > 0 {
				st.NaggedAt = c.nowPtr()
			}
		}
		if len(lines) == 0 && c.unheard() && (st.NaggedAt == nil || now.Sub(*st.NaggedAt) >= w.Nag) {
			lines = c.nag()
		}
	})
	if stop {
		return false
	}
	if len(lines) > 0 {
		if w.Write(lines) != nil {
			return false
		}
		if w.Notify != nil {
			w.Notify(lines)
		}
	}
	return true
}

// sightings are the pods and nodes she saw, and the API server itself.
func (s Snapshot) sightings() []Sighting {
	api := Sighting{Object: apiserver, Healthy: true}
	if s.APIError != "" {
		api = Sighting{Object: apiserver, Trouble: true, Why: "the API server is " + s.APIError}
	}
	return append(append([]Sighting{api}, s.Pods...), s.Nodes...)
}

var (
	unreachable = regexp.MustCompile(`(?i)unable to connect|connection refused|i/o timeout|no such host|no route to host|handshake timeout|deadline exceeded|connection reset|\bEOF\b`)
	serverError = regexp.MustCompile(`(?i)\((InternalError|ServiceUnavailable)\)|internal server error|status code 5\d\d|currently unable to handle`)
)

// apiError tells an API server that is gone or failing from any other
// error (forbidden, not found), which is not hers to worry about.
func apiError(stderr string) string {
	switch {
	case serverError.MatchString(stderr):
		return "returning server errors"
	case unreachable.MatchString(stderr):
		return "unreachable"
	}
	return ""
}

// look is one round of her looking: a handful of read-only calls.
func (w *Watcher) look() Snapshot {
	var snap Snapshot
	scope := []string{}
	if w.All {
		scope = []string{"-A"}
	}
	get := func(what ...string) (string, bool) {
		args := append(append([]string{"--context", w.Context, "--request-timeout", "10s", "get"}, what...), scope...)
		if what[0] == "nodes" {
			args = args[:len(args)-len(scope)]
		}
		if !readOnlyCall(args) {
			return "", false
		}
		out, errOut, code := w.Run(args)
		if code != 0 {
			if snap.APIError == "" {
				snap.APIError = apiError(errOut)
			}
			return "", false
		}
		return out, true
	}
	ls := []string{"get", "pods"}
	if w.All {
		ls = append(ls, "-A")
	}
	if out, ok := get("pods"); ok {
		snap.Pods = Notice(ls, out, w.Now())
	}
	if snap.APIError != "" {
		return snap
	}
	if out, ok := get("nodes"); ok {
		snap.Nodes = Notice([]string{"get", "nodes"}, out, w.Now())
	}
	if out, ok := get("events", "--field-selector", "type=Warning"); ok {
		snap.Events = parseEvents(out, "")
	}
	if out, ok := get("pvc"); ok {
		snap.PVCs = parsePVCs(out, "")
	}
	if out, ok := get("deployments", "-o", "json"); ok {
		snap.Deploys = parseDeploys(out, w.All, w.Now())
	}
	return snap
}

// kubectlRunner runs one read-only call with a timeout and bounded output.
func kubectlRunner(kubectl string) func(args []string) (string, string, int) {
	return func(args []string) (string, string, int) {
		if !readOnlyCall(args) {
			return "", "misogynectl: refused a call that is not read-only", 1
		}
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, kubectl, args...)
		d, _ := strconv.Atoi(os.Getenv(depthVar))
		cmd.Env = append(os.Environ(), depthVar+"="+strconv.Itoa(d+1))
		out, errOut := &capped{max: maxCallOutput}, &capped{max: 8 << 10}
		cmd.Stdout, cmd.Stderr = out, errOut
		cmd.WaitDelay = waitDelay
		err := cmd.Run()
		code := 0
		if err != nil {
			code = 1
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() > 0 {
				code = cmd.ProcessState.ExitCode()
			}
			if ctx.Err() != nil {
				errOut.Write([]byte("\ncontext deadline exceeded"))
			}
		}
		return string(out.b), string(errOut.b), code
	}
}

// capped keeps the first max bytes written to it.
type capped struct {
	b   []byte
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - len(c.b); room > 0 {
		c.b = append(c.b, p[:min(len(p), room)]...)
	}
	return len(p), nil
}

// envDuration reads a duration from the environment, never below floor.
func envDuration(name string, def, floor time.Duration) time.Duration {
	d, err := time.ParseDuration(os.Getenv(name))
	if err != nil {
		return def
	}
	return max(d, floor)
}

// lifetime is maxLifetime, or shorter when MISOGYNETES_WATCH_LIFETIME
// says so; never longer.
func lifetime() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("MISOGYNETES_WATCH_LIFETIME")); err == nil && d > 0 && d < maxLifetime {
		return d
	}
	return maxLifetime
}

// watchKey names a watcher's lock and pid files after its context.
func watchKey(dir, context string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(context))
	return filepath.Join(dir, fmt.Sprintf("watch-%x", h.Sum64()))
}

// flagValue is the value of a flag on the command line, before "--".
func flagValue(args []string, names ...string) string {
	for i, a := range args {
		if a == "--" {
			return ""
		}
		for _, n := range names {
			switch {
			case a == n && i+1 < len(args):
				return args[i+1]
			case strings.HasPrefix(a, n+"="):
				return strings.TrimPrefix(a, n+"=")
			}
		}
	}
	return ""
}

// kubeconfigPath is the kubeconfig kubectl would read first.
func kubeconfigPath(args []string) string {
	if p := flagValue(args, "--kubeconfig"); p != "" {
		return p
	}
	if list := filepath.SplitList(os.Getenv("KUBECONFIG")); len(list) > 0 && list[0] != "" {
		return list[0]
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".kube", "config")
	}
	return ""
}

// currentContext is the context a command is for: the one typed, or the
// kubeconfig's current one. It is read from the file; kubectl is not asked.
func currentContext(args []string) string {
	if c := flagValue(args, "--context"); c != "" {
		return c
	}
	f, err := os.Open(kubeconfigPath(args))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "current-context:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// ttyLines is what she writes to the terminal: a newline first, so the
// prompt is only a little mangled.
func ttyLines(lines []string) string {
	var b strings.Builder
	b.WriteString("\n")
	for _, l := range lines {
		fmt.Fprintf(&b, "\033[35m%s\033[0m\n", l)
	}
	return b.String()
}

// notify shows the lines as a macOS notification.
func notify(lines []string) {
	msg := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(strings.Join(lines, " "))
	_ = exec.Command("osascript", "-e", `display notification "`+msg+`" with title "misogynectl"`).Run()
}

// LeaveMeAlone stops her watcher for good, until "come-back". She takes it
// personally.
func (c *Cluster) LeaveMeAlone() []string {
	c.State.LeftAlone = true
	return []string{c.pick(leaveLines), leftAloneHint}
}

// ComeBack lets her watch again from the next command.
func (c *Cluster) ComeBack() []string {
	if !c.State.LeftAlone {
		return []string{neverLeft}
	}
	c.State.LeftAlone = false
	return []string{cameBack}
}
