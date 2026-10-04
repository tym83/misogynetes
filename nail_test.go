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
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	crashTable = `NAME     READY   STATUS             RESTARTS      AGE
web-1    0/1     CrashLoopBackOff   6 (40s ago)   9m
db-0     1/1     Running            0             3d
`
	healthyTable = `NAME     READY   STATUS    RESTARTS      AGE
web-1    1/1     Running   6 (25m ago)   40m
db-0     1/1     Running   0             3d
`
)

var (
	getPods = []string{"get", "pods", "-n", "shop"}
	delWeb  = []string{"delete", "pod", "web-1", "-n", "shop"}
)

func contains(lines []string, s string) bool {
	return strings.Contains(strings.Join(lines, "\n"), s)
}

func TestSheSharesOnceAndRemembers(t *testing.T) {
	c := cluster(3)
	lines := c.Observe(getPods, crashTable)
	if len(lines) != 2 || !contains(lines, "pod/web-1 in shop") || !contains(lines, "CrashLoopBackOff") ||
		!contains(lines, "misogynectl aga") || !contains(lines, "exit 75") {
		t.Fatalf("share: %q", lines)
	}
	if len(c.State.Shared) != 1 || c.State.Shared[0].Object != "shop/pod/web-1" || c.State.Shared[0].Heard {
		t.Fatalf("state: %+v", c.State.Shared)
	}
	if again := c.Observe(getPods, crashTable); len(again) != 0 {
		t.Errorf("shared the same nail twice: %q", again)
	}
	c.Now = c.Now.Add(shareWindow + time.Minute)
	if again := c.Observe(getPods, crashTable); !contains(again, "web-1") {
		t.Errorf("did not bring it up again after the window: %q", again)
	}
}

func TestFixingIsRefusedThenInsistedOn(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)

	p := c.Before(delWeb)
	if p.Run || p.Code != nailExit || !contains(p.Say, "nothing ran, exit 75") {
		t.Fatalf("first fix: %+v", p)
	}
	if c.State.Hurt == nil || c.State.Hurt.Command != "delete pod web-1" {
		t.Fatalf("hurt: %+v", c.State.Hurt)
	}
	// A different fix is a new attempt, not insisting.
	if p := c.Before([]string{"rollout", "restart", "deploy/web"}); p.Run || p.Code != nailExit {
		t.Fatalf("other fix: %+v", p)
	}
	if p := c.Before(delWeb); p.Run {
		t.Fatalf("insisting on an older fix ran it: %+v", p)
	}
	c.Now = c.Now.Add(insistWindow / 2)
	p = c.Before(delWeb)
	if !p.Run || p.Say[len(p.Say)-1] != insisted {
		t.Fatalf("insisting: %+v", p)
	}
	if c.State.Shared != nil || c.State.Hurt != nil || c.State.Curt != curtRuns {
		t.Fatalf("after insisting: %+v", c.State)
	}
	for i := 0; i < curtRuns; i++ {
		p := c.Before(getPods)
		if !p.Run || len(p.Say) != 1 || !contains(curtLines, p.Say[0]) {
			t.Fatalf("curt reply %d: %+v", i, p)
		}
	}
	if c.State.Curt != 0 {
		t.Errorf("still curt: %d", c.State.Curt)
	}
}

func TestInsistingTooLateIsAnotherRefusal(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	c.Before(delWeb)
	c.Now = c.Now.Add(insistWindow + time.Second)
	if p := c.Before(delWeb); p.Run || p.Code != nailExit {
		t.Errorf("late insist ran: %+v", p)
	}
}

func TestReadsAndHeardProblemsAreNotRefused(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	for _, args := range [][]string{getPods, {"describe", "pod", "web-1"}, {"logs", "web-1"}, {"rollout", "status", "deploy/web"}} {
		if p := c.Before(args); p.Code == nailExit {
			t.Errorf("%v refused: %+v", args, p)
		}
	}
	if got := c.Aga(); !contains(heardLines, got[0]) {
		t.Fatalf("aga: %q", got)
	}
	if p := c.Before(delWeb); p.Code == nailExit || c.State.Hurt != nil {
		t.Errorf("fix after listening refused: %+v", p)
	}
}

func TestOldProblemsAreNotRefused(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	c.Now = c.Now.Add(shareWindow)
	if p := c.Before(delWeb); p.Code == nailExit {
		t.Errorf("refused after the window: %+v", p)
	}
}

func TestAgaResolvesTheSulk(t *testing.T) {
	c := cluster(3)
	if got := c.Aga(); got[0] != agaNothing {
		t.Errorf("aga on nothing: %q", got)
	}
	c.Observe(getPods, crashTable)
	c.Before(delWeb)
	if got := c.Aga(); got[0] != sulkOver || c.State.Hurt != nil {
		t.Errorf("aga after a refusal: %q %+v", got, c.State)
	}
	if got := c.Aga(); got[0] != stillHeard {
		t.Errorf("aga again: %q", got)
	}

	c = cluster(3)
	c.Observe(getPods, crashTable)
	c.Before(delWeb)
	c.Before(delWeb)
	if got := c.Aga(); got[0] != sulkOver || c.State.Curt != 0 {
		t.Errorf("aga while curt: %q %+v", got, c.State)
	}
	if p := c.Before(getPods); len(p.Say) == 1 && contains(curtLines, p.Say[0]) {
		t.Errorf("still curt after aga: %+v", p)
	}
}

func TestItSortsItselfOut(t *testing.T) {
	// Only listened: thanks, and the bonus.
	c := cluster(3)
	c.Observe(getPods, crashTable)
	c.Aga()
	lines := c.Observe(getPods, healthyTable)
	if len(lines) != 2 || !contains(lines, "pod/web-1 in shop sorted itself out") || !contains(onlyListened, lines[1]) {
		t.Errorf("heard, sorted out: %q", lines)
	}
	if len(c.State.Shared) != 0 {
		t.Errorf("still remembered: %+v", c.State.Shared)
	}

	// Never listened: thanks, no bonus. "get pods" without -n is the same pod.
	c = cluster(3)
	c.Observe(getPods, crashTable)
	lines = c.Observe([]string{"get", "pods"}, healthyTable)
	if len(lines) != 1 || !contains(lines, "sorted itself out") {
		t.Errorf("unheard, sorted out: %q", lines)
	}
	if len(c.State.Shared) != 0 {
		t.Errorf("still remembered: %+v", c.State.Shared)
	}
}

func TestSheForgetsAndKeepsLittle(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	c.Now = c.Now.Add(forgetAfter + time.Minute)
	c.Observe([]string{"get", "pods"}, "No resources found.\n")
	if len(c.State.Shared) != 0 {
		t.Errorf("never forgets: %+v", c.State.Shared)
	}
	var table strings.Builder
	table.WriteString("NAME     READY   STATUS   RESTARTS   AGE\n")
	for i := 0; i < 40; i++ {
		table.WriteString("web-" + strconv.Itoa(i+10) + "   0/1     Error    0          1m\n")
	}
	lines := c.Observe(getPods, table.String())
	if len(c.State.Shared) != maxShared || !contains(lines, "And 39 more") {
		t.Errorf("kept %d, said %q", len(c.State.Shared), lines)
	}
}

func TestApologyKeepsTheNail(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	c.Failed([]string{"get", "pods"}, "boom")
	c.Sorry("the get")
	if len(c.State.Shared) != 1 {
		t.Errorf("an apology made her forget what she shared: %+v", c.State)
	}
}

func TestPhrasesAreSeeded(t *testing.T) {
	say := func() string {
		c := cluster(3)
		c.Observe(getPods, crashTable)
		return strings.Join(c.Before(delWeb).Say, "\n")
	}
	if a, b := say(), say(); a != b {
		t.Errorf("same seed, different words: %q vs %q", a, b)
	}
}

func TestAgaSynonyms(t *testing.T) {
	for _, w := range []string{"aga", "ага", "угу", "uh-huh", "mhm", "aha", "yeah"} {
		if own, _ := ownCommand([]string{w}); own != "aga" {
			t.Errorf("%q is not aga: %q", w, own)
		}
	}
	if own, _ := ownCommand([]string{"--as", "aga", "get", "pods"}); own != "" {
		t.Errorf("flag value taken as aga")
	}
}

// --- end to end, with a fake kubectl on the PATH ---

// nailKubectl logs every call and prints the table in $TABLE for get.
const nailKubectl = `echo "$*" >> "$0.log"
case "$1" in
get) cat "$TABLE" ;;
delete) echo "pod \"$3\" deleted"; exit "${DELETE_EXIT:-0}" ;;
esac
`

// kubectlCalls is every command the fake kubectl was given.
func (s *sandbox) kubectlCalls() []string {
	b, err := os.ReadFile(s.fake + ".log")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		s.t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func (s *sandbox) table(name, content string) string {
	s.t.Helper()
	path := filepath.Join(s.dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		s.t.Fatal(err)
	}
	return "TABLE=" + path
}

func plain(s string) string {
	return strings.NewReplacer("\033[35m", "", "\033[0m", "").Replace(s)
}

func TestNailEndToEnd(t *testing.T) {
	s := newSandbox(t, nailKubectl)
	crash, healthy := s.table("crash", crashTable), s.table("healthy", healthyTable)
	// Her answers to a fix and while curt come before any whim.
	nail := []string{"MISOGYNETES=always", "MISOGYNETES_DAY=3", "MISOGYNETES_SEED=0", "DELETE_EXIT=7"}

	out, errOut, code := s.acting([]string{crash}, getPods...)
	if out != crashTable || code != 0 || !strings.Contains(plain(errOut), "pod/web-1 in shop") {
		t.Fatalf("share: stdout %q stderr %q code %d", out, errOut, code)
	}

	calls := len(s.kubectlCalls())
	out, errOut, code = s.run(append(nail, crash), delWeb...)
	if code != nailExit || out != "" || !strings.Contains(plain(errOut), "nothing ran, exit 75") {
		t.Fatalf("refusal: stdout %q stderr %q code %d", out, errOut, code)
	}
	if len(s.kubectlCalls()) != calls {
		t.Fatalf("kubectl ran on a refusal: %q", s.kubectlCalls())
	}

	out, errOut, code = s.run(append(nail, crash), delWeb...)
	if code != 7 || out != "pod \"web-1\" deleted\n" || !strings.Contains(plain(errOut), insisted) {
		t.Fatalf("insist: stdout %q stderr %q code %d", out, errOut, code)
	}

	out, errOut, code = s.run(append(nail, crash), getPods...)
	if code != 0 || out != crashTable || !contains(curtLines, strings.SplitN(plain(errOut), "\n", 2)[0]) {
		t.Fatalf("curt: stdout %q stderr %q code %d", out, errOut, code)
	}

	calls = len(s.kubectlCalls())
	if _, errOut, code := s.run(nail, "угу"); code != 0 || !strings.Contains(plain(errOut), sulkOver) {
		t.Fatalf("aga: stderr %q code %d", errOut, code)
	}
	if len(s.kubectlCalls()) != calls {
		t.Fatal("aga ran kubectl")
	}

	_, errOut, _ = s.acting([]string{healthy}, getPods...)
	if !strings.Contains(plain(errOut), "sorted itself out") {
		t.Fatalf("resolution: %q", errOut)
	}

	// kubectl ran exactly the commands typed, in order, and nothing else.
	want := []string{strings.Join(getPods, " "), strings.Join(delWeb, " "), strings.Join(getPods, " ")}
	got := s.kubectlCalls()
	if len(got) < len(want)+1 || strings.Join(got[:3], "|") != strings.Join(want, "|") {
		t.Errorf("kubectl calls: %q", got)
	}
	for _, c := range got[3:] {
		if c != strings.Join(getPods, " ") {
			t.Errorf("unexpected kubectl call %q", c)
		}
	}

	var st State
	b, err := os.ReadFile(s.statePath())
	if err != nil || json.Unmarshal(b, &st) != nil {
		t.Fatalf("state: %v %s", err, b)
	}
	if len(st.Shared) != 0 || st.Hurt != nil || st.Curt != 0 {
		t.Errorf("left over: %s", b)
	}
}

// TestNailNeverBlocksPipes: with a fix pending, in a pipe or with her off,
// she is plain kubectl, byte for byte.
func TestNailNeverBlocksPipes(t *testing.T) {
	s := newSandbox(t, nailKubectl)
	raw := "NAME  READY  STATUS  RESTARTS  AGE\nweb-1  0/1  Error  0  1m\n\x00\xff no newline at the end"
	tbl := s.table("raw", raw)
	now := time.Now()
	writeState(t, s.statePath(), &State{Installed: now, BannerShown: true,
		Shared: []Share{{Object: "shop/pod/web-1", Why: "it's Error", At: now}},
		Hurt:   &Hurt{Command: "scale deploy web", At: now}, Curt: 0})
	before, err := os.ReadFile(s.statePath())
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"", "off"} {
		env := []string{"MISOGYNETES=" + mode, tbl, "DELETE_EXIT=7"}
		out, errOut, code := s.run(env, delWeb...)
		if out != "pod \"web-1\" deleted\n" || errOut != "" || code != 7 {
			t.Errorf("%q delete: stdout %q stderr %q code %d", mode, out, errOut, code)
		}
		out, errOut, code = s.run(env, getPods...)
		if out != raw || errOut != "" || code != 0 {
			t.Errorf("%q get: stdout %q stderr %q code %d", mode, out, errOut, code)
		}
		// "aga" is kubectl's to answer here, like any other word.
		if out, errOut, code := s.run(env, "aga"); out != "" || errOut != "" || code != 0 {
			t.Errorf("%q aga: stdout %q stderr %q code %d", mode, out, errOut, code)
		}
	}
	if got := s.kubectlCalls(); len(got) != 6 || got[0] != strings.Join(delWeb, " ") || got[2] != "aga" {
		t.Errorf("kubectl calls: %q", got)
	}
	if after, _ := os.ReadFile(s.statePath()); string(after) != string(before) {
		t.Errorf("state touched in a pipe:\n%s\n%s", before, after)
	}
}

// TestTeeIsByteIdentical: when she reads along, kubectl's stdout still
// reaches you unchanged.
func TestTeeIsByteIdentical(t *testing.T) {
	s := newSandbox(t, nailKubectl)
	raw := crashTable + strings.Repeat("filler-row-that-is-long-enough  1/1  Running  0  1d\n", 20000) + "\x00\xfftail"
	out, _, code := s.acting([]string{s.table("big", raw)}, getPods...)
	if out != raw || code != 0 {
		t.Errorf("tee changed stdout: %d bytes vs %d, code %d", len(out), len(raw), code)
	}
}
