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
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	calmPods = `NAME    READY   STATUS    RESTARTS   AGE
web-1   1/1     Running   0          1d
`
	noEvents = "No resources found in default namespace.\n"
	noPVCs   = "No resources found in default namespace.\n"
	noDeploy = `{"items": []}`
)

// fakeCluster answers her read-only calls from fixtures and records them.
type fakeCluster struct {
	out   map[string]string // resource -> stdout
	fail  string            // stderr for every call when set
	calls [][]string
}

func (f *fakeCluster) run(args []string) (string, string, int) {
	f.calls = append(f.calls, args)
	if f.fail != "" {
		return "", f.fail, 1
	}
	return f.out[Words(args)[1]], "", 0
}

func calmCluster() *fakeCluster {
	return &fakeCluster{out: map[string]string{"pods": calmPods, "nodes": readyNodes,
		"events": noEvents, "pvc": noPVCs, "deployments": noDeploy}}
}

// testWatcher is a Watcher on a fake cluster with a clock the test moves.
type testWatcher struct {
	*Watcher
	clock   time.Time
	cluster *fakeCluster
	said    [][]string
	alive   bool
}

func newTestWatcher(t *testing.T, f *fakeCluster) *testWatcher {
	tw := &testWatcher{clock: t0, cluster: f, alive: true}
	tw.Watcher = &Watcher{Context: "prod", StatePath: filepath.Join(t.TempDir(), "state.json"),
		Rand: rand.New(rand.NewSource(1)), Day: 3, Poll: defaultPoll, Nag: defaultNag,
		Lifetime: maxLifetime, Started: t0,
		Now:   func() time.Time { return tw.clock },
		Run:   f.run,
		Alive: func() bool { return tw.alive },
		Write: func(lines []string) error { tw.said = append(tw.said, lines); return nil },
	}
	return tw
}

// at moves the clock and runs one tick; it returns what she wrote.
func (tw *testWatcher) at(t *testing.T, d time.Duration) []string {
	t.Helper()
	tw.clock = t0.Add(d)
	before := len(tw.said)
	if !tw.Tick() {
		t.Fatalf("at %v: she stopped", d)
	}
	if len(tw.said) == before {
		return nil
	}
	return tw.said[len(tw.said)-1]
}

func (tw *testWatcher) state(t *testing.T) *State {
	t.Helper()
	s, ok := Load(tw.StatePath, tw.clock, tw.Rand)
	if !ok {
		t.Fatal("state unreadable")
	}
	return s
}

func (tw *testWatcher) update(f func(c *Cluster)) {
	Update(tw.StatePath, tw.clock, tw.Rand, func(s *State) {
		f(&Cluster{Rand: tw.Rand, Now: tw.clock, Day: 3, State: s})
	})
}

func TestReadOnlyCall(t *testing.T) {
	for _, tc := range []struct {
		args []string
		ok   bool
	}{
		{[]string{"--context", "prod", "get", "pods"}, true},
		{[]string{"--context", "prod", "--request-timeout", "10s", "get", "events", "--field-selector", "type=Warning", "-A"}, true},
		{[]string{"--context", "prod", "get", "deployments", "-o", "json"}, true},
		{[]string{"get", "pods"}, false},
		{[]string{"--context", "", "get", "pods"}, false},
		{[]string{"--context", "prod", "get", "secrets"}, false},
		{[]string{"--context", "prod", "get", "configmaps"}, false},
		{[]string{"--context", "prod", "get", "pods", "web-1"}, false},
		{[]string{"--context", "prod", "delete", "pods"}, false},
		{[]string{"--context", "prod", "logs", "web-1"}, false},
	} {
		if got := readOnlyCall(tc.args); got != tc.ok {
			t.Errorf("readOnlyCall(%v) = %v", tc.args, got)
		}
	}
}

func TestWatcherOnlyReadsWithItsContext(t *testing.T) {
	f := calmCluster()
	tw := newTestWatcher(t, f)
	tw.All = true
	tw.at(t, 0)
	if len(f.calls) != 5 {
		t.Fatalf("%d calls: %q", len(f.calls), f.calls)
	}
	for _, c := range f.calls {
		if !readOnlyCall(c) || c[1] != "prod" {
			t.Errorf("call %q", c)
		}
	}
	// Between polls she reads her state, not the cluster.
	tw.at(t, checkEvery)
	tw.at(t, 2*checkEvery)
	if len(f.calls) != 10 {
		t.Errorf("polled %d times, want 2", len(f.calls)/5)
	}
}

func TestWatcherComesRunningAndEscalates(t *testing.T) {
	f := calmCluster()
	f.out["pods"] = crashTable
	tw := newTestWatcher(t, f)
	tw.Poll = time.Hour // only the first look matters here

	said := tw.at(t, 0)
	if !contains(said, "pod/web-1") || !contains(said, "misogynectl aga") {
		t.Fatalf("first look: %q", said)
	}
	if got := tw.at(t, time.Minute); got != nil {
		t.Errorf("nagged after a minute: %q", got)
	}
	// Every Nag minutes, louder, for as long as nobody says aga.
	for level := 1; level <= 6; level++ {
		said := tw.at(t, time.Duration(level)*defaultNag)
		if len(said) != 1 {
			t.Fatalf("level %d: %q", level, said)
		}
		if level <= len(reminders) && !strings.Contains(said[0], "pod/web-1") {
			t.Errorf("level %d does not name it: %q", level, said)
		}
		if st := tw.state(t); st.Ignored != level {
			t.Errorf("level %d: ignored %d", level, st.Ignored)
		}
	}
	tw.update(func(c *Cluster) { c.Aga() })
	for d := 6*defaultNag + checkEvery; d < tw.Poll; d += checkEvery {
		if got := tw.at(t, d); got != nil {
			t.Fatalf("nagged %v after aga: %q", d-6*defaultNag, got)
		}
	}
	// Heard a while ago and still crashing: that is news again.
	if got := tw.at(t, tw.Poll); !contains(got, "pod/web-1") {
		t.Errorf("still crashing an hour later, not a word: %q", got)
	}
}

func TestNagIntervalHasAFloor(t *testing.T) {
	t.Setenv("MISOGYNETES_NAG", "5s")
	if got := envDuration("MISOGYNETES_NAG", defaultNag, minNag); got != minNag {
		t.Errorf("nag %v", got)
	}
	t.Setenv("MISOGYNETES_POLL", "1ms")
	if got := envDuration("MISOGYNETES_POLL", defaultPoll, minPoll); got != minPoll {
		t.Errorf("poll %v", got)
	}
	t.Setenv("MISOGYNETES_NAG", "7m")
	if got := envDuration("MISOGYNETES_NAG", defaultNag, minNag); got != 7*time.Minute {
		t.Errorf("nag %v", got)
	}
	t.Setenv("MISOGYNETES_WATCH_LIFETIME", "100h")
	if lifetime() != maxLifetime {
		t.Error("lifetime made longer")
	}
}

func TestWatcherStops(t *testing.T) {
	for name, stop := range map[string]func(t *testing.T, tw *testWatcher){
		"lifetime":       func(_ *testing.T, tw *testWatcher) { tw.clock = t0.Add(maxLifetime) },
		"tty gone":       func(_ *testing.T, tw *testWatcher) { tw.alive = false },
		"leave me alone": func(_ *testing.T, tw *testWatcher) { tw.update(func(c *Cluster) { c.LeaveMeAlone() }) },
		"off":            func(t *testing.T, _ *testWatcher) { t.Setenv("MISOGYNETES", "off") },
	} {
		t.Run(name, func(t *testing.T) {
			tw := newTestWatcher(t, calmCluster())
			tw.at(t, 0)
			tw.clock = t0.Add(time.Hour)
			stop(t, tw)
			if tw.Tick() {
				t.Error("she went on")
			}
		})
	}
}

func TestUnreachableIsAComplaintWithBackoff(t *testing.T) {
	f := calmCluster()
	f.fail = "Unable to connect to the server: dial tcp 10.0.0.1:6443: connect: connection refused"
	tw := newTestWatcher(t, f)
	said := tw.at(t, 0)
	if !contains(said, "apiserver") || !contains(said, talkLines[0]) && !contains(said, talkLines[1]) && !contains(said, talkLines[2]) {
		t.Fatalf("unreachable: %q", said)
	}
	if len(f.calls) != 1 {
		t.Errorf("kept calling an unreachable server: %d calls", len(f.calls))
	}
	if strings.Contains(fmt.Sprint(tw.state(t)), "10.0.0.1") {
		t.Error("kept the error text")
	}
	// Backoff: the next look waits longer than Poll.
	tw.at(t, tw.Poll)
	if len(f.calls) != 1 {
		t.Errorf("no backoff: %d calls", len(f.calls))
	}
	tw.at(t, 2*tw.Poll)
	if len(f.calls) != 2 {
		t.Errorf("never looked again: %d calls", len(f.calls))
	}
	// Back: it sorted itself out, and the talk is off.
	f.fail = ""
	said = tw.at(t, 10*tw.Poll)
	if !contains(said, forgetIt) || !contains(said, "sorted itself out") {
		t.Errorf("back: %q", said)
	}
}

func TestWeNeedToTalkThenWhatThreeTimes(t *testing.T) {
	f := calmCluster()
	f.out["nodes"] = notReadyNodes
	tw := newTestWatcher(t, f)
	said := tw.at(t, 0)
	if !contains(said, talkLines[0]) && !contains(said, talkLines[1]) && !contains(said, talkLines[2]) {
		t.Fatalf("no talk: %q", said)
	}
	// A fix during the talk follows the nail's rules.
	tw.update(func(c *Cluster) {
		if p := c.Before([]string{"--context", "prod", "drain", "node-2"}); p.Code != nailExit {
			t.Errorf("drain during the talk: %+v", p)
		}
	})

	var answers [][]string
	for i := 0; i < 3; i++ {
		tw.clock = t0.Add(time.Duration(i+1) * time.Minute)
		tw.update(func(c *Cluster) { answers = append(answers, c.What()) })
	}
	if answers[0][0] != talkWhatAnswers[0] || answers[1][0] != talkWhatAnswers[1] {
		t.Errorf("first two: %q", answers[:2])
	}
	summary := strings.Join(answers[2], "\n")
	for _, want := range []string{"Context prod", "node/node-2: node NotReady", "since " + t0.Local().Format("15:04"),
		"kubectl --context prod describe node node-2"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary without %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "node-1") {
		t.Errorf("summary blames a healthy node:\n%s", summary)
	}
	if st := tw.state(t); st.Talk == nil || !st.Talk.Done {
		t.Errorf("talk not done: %+v", st.Talk)
	}

	// Healthy again: "Forget it." and the talk is over.
	f.out["nodes"] = readyNodes
	said = tw.at(t, tw.Poll)
	if !contains(said, forgetIt) {
		t.Errorf("healthy again: %q", said)
	}
	if st := tw.state(t); st.Talk != nil {
		t.Errorf("talk left over: %+v", st.Talk)
	}
}

func TestCurrentContext(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	if err := os.WriteFile(cfg, []byte("apiVersion: v1\nkind: Config\ncurrent-context: \"admin@prod\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", cfg+string(filepath.ListSeparator)+"/nope")
	if got := currentContext([]string{"get", "pods"}); got != "admin@prod" {
		t.Errorf("from KUBECONFIG: %q", got)
	}
	if got := currentContext([]string{"--context=dev", "get", "pods"}); got != "dev" {
		t.Errorf("typed: %q", got)
	}
	if got := currentContext([]string{"--kubeconfig", filepath.Join(dir, "missing"), "get"}); got != "" {
		t.Errorf("missing kubeconfig: %q", got)
	}
}
