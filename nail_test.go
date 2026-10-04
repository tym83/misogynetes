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
	"fmt"
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

const nsTable = "NAME      STATUS   AGE\ndefault   Active   9d\n"

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
	// Unheard, the same trouble is never announced as new again: she
	// reminds you instead, however long it takes.
	for _, later := range []time.Duration{0, shareWindow + time.Minute, 5 * time.Hour} {
		c.Now = t0.Add(later)
		if again := c.Observe(getPods, crashTable); len(again) != 0 {
			t.Errorf("after %v: announced the same nail again: %q", later, again)
		}
	}
	// Heard, the same trouble is news again only after the window.
	c.Now = t0
	c.Aga()
	if again := c.Observe(getPods, crashTable); len(again) != 0 {
		t.Errorf("announced right after aga: %q", again)
	}
	c.Now = t0.Add(shareWindow + time.Minute)
	if again := c.Observe(getPods, crashTable); !contains(again, "web-1") || !c.unheard() {
		t.Errorf("heard trouble back after the window: %q", again)
	}
}

// remind runs one ordinary command past her and returns what she said.
func remind(t *testing.T, c *Cluster, args []string) []string {
	t.Helper()
	p := c.Before(args)
	if !p.Run || p.Code != 0 {
		t.Fatalf("%v did not run: %+v", args, p)
	}
	return p.Say
}

func TestSheEscalatesUntilHeard(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	commands := [][]string{{"get", "ns"}, {"top", "nodes"}, {"get", "svc", "-n", "other"},
		getPods, {"version"}, {"describe", "node", "n1"}, {"api-resources"}, {"get", "cm"}}
	for i, args := range commands {
		c.Now = t0.Add(time.Duration(i) * 10 * time.Minute) // well past 15 minutes
		said := remind(t, c, args)
		if len(said) != 1 {
			t.Fatalf("command %d: %q", i, said)
		}
		level := i + 1
		var table []string
		switch {
		case level <= len(reminders):
			table = reminders[level-1]
		case level%2 == 0:
			table = neverMind
		default:
			table = pointedReminders
		}
		var want []string
		for _, l := range table {
			want = append(want, strings.ReplaceAll(l, "{obj}", "pod/web-1 in shop"))
		}
		if !contains(want, said[0]) {
			t.Errorf("level %d: %q not from its table", level, said[0])
		}
		if c.State.Ignored != level {
			t.Errorf("level %d: ignored %d", level, c.State.Ignored)
		}
	}
	if got := c.Aga(); !contains(heardLines, got[0]) || c.State.Ignored != 0 || c.unheard() {
		t.Fatalf("aga: %q %+v", got, c.State)
	}
	if p := c.Before([]string{"get", "ns"}); len(p.Say) > 1 || (len(p.Say) == 1 && strings.Contains(p.Say[0], "web-1")) {
		t.Errorf("reminded after aga: %q", p.Say)
	}
}

func TestRemindersNameEveryoneWaiting(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, "NAME  READY  STATUS  RESTARTS  AGE\na-1   0/1    Error   0         1m\nb-1   0/1    Error   0         1m\nc-1   0/1    Error   0         1m\n")
	said := remind(t, c, []string{"get", "ns"})
	if !contains(said, "pod/a-1 in shop, pod/b-1 in shop and 1 more") {
		t.Errorf("reminder: %q", said)
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
	if c.State.Hurt != nil || !c.State.Sulking || !c.unheard() {
		t.Fatalf("after insisting: %+v", c.State)
	}
	// She sulks, and still wants to be heard, for as long as it takes.
	for i := 0; i < 10; i++ {
		c.Now = c.Now.Add(time.Hour)
		said := remind(t, c, []string{"get", "ns"})
		if len(said) != 2 || !contains(curtLines, said[0]) {
			t.Fatalf("command %d after insisting: %q", i, said)
		}
	}
	if got := c.Aga(); got[0] != sulkOver || c.State.Sulking || c.State.Ignored != 0 {
		t.Fatalf("aga: %q %+v", got, c.State)
	}
	if p := c.Before([]string{"get", "ns"}); len(p.Say) == 1 && contains(curtLines, p.Say[0]) {
		t.Errorf("still curt after aga: %+v", p)
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
		if p := c.Before(args); p.Code == nailExit || !p.Run {
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

func TestUnheardDoesNotExpire(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	c.Now = c.Now.Add(forgetAfter - time.Minute)
	if p := c.Before(delWeb); p.Code != nailExit {
		t.Errorf("not refused after %v: %+v", forgetAfter-time.Minute, p)
	}
	// The safety valve: a day later she lets go.
	c.Now = t0.Add(forgetAfter + time.Minute)
	if p := c.Before(delWeb); p.Code == nailExit || c.unheard() || c.State.Ignored != 0 {
		t.Errorf("not forgotten after a day: %+v %+v", p, c.State)
	}
}

func TestAgaAnswers(t *testing.T) {
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
}

func TestHealedButUnheard(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	// "get pods" without -n is the same pod.
	lines := c.Observe([]string{"get", "pods"}, healthyTable)
	if len(lines) != 1 || lines[0] != fmt.Sprintf(sortedUnheard, "pod/web-1 in shop") {
		t.Fatalf("healed, unheard: %q", lines)
	}
	if again := c.Observe(getPods, healthyTable); len(again) != 0 {
		t.Errorf("said it twice: %q", again)
	}
	if !c.unheard() {
		t.Fatal("healing made her heard")
	}
	if said := remind(t, c, []string{"get", "ns"}); !contains(said, "web-1") {
		t.Errorf("no reminder after healing: %q", said)
	}
	if p := c.Before(delWeb); p.Code != nailExit {
		t.Errorf("fix not refused while unheard: %+v", p)
	}
	got := c.Aga()
	if len(got) != 2 || !contains(got, fmt.Sprintf(sortedItself, "pod/web-1 in shop")) {
		t.Errorf("aga after healing: %q", got)
	}
	if len(c.State.Shared) != 0 || c.unheard() {
		t.Errorf("left over: %+v", c.State)
	}

	// It broke again before she was heard: still the same complaint.
	c = cluster(3)
	c.Observe(getPods, crashTable)
	c.Observe(getPods, healthyTable)
	if again := c.Observe(getPods, crashTable); len(again) != 0 || c.State.Shared[0].Healed {
		t.Errorf("relapse: %q %+v", again, c.State.Shared)
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
get) if [ "$2" = pods ]; then cat "$TABLE"; else printf 'NAME      STATUS   AGE\ndefault   Active   9d\n'; fi ;;
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
	// While she waits to be heard, her words come before any whim, so
	// every command here runs (or is refused) the same way for any seed.
	nail := []string{"MISOGYNETES=always", "MISOGYNETES_DAY=3", "MISOGYNETES_SEED=0", "DELETE_EXIT=7", crash}
	getNS := []string{"get", "ns"}
	var typed []string

	out, errOut, code := s.acting([]string{crash}, getPods...)
	if out != crashTable || code != 0 || !strings.Contains(plain(errOut), "pod/web-1 in shop") {
		t.Fatalf("share: stdout %q stderr %q code %d", out, errOut, code)
	}
	typed = append(typed, strings.Join(getPods, " "))

	// Ignored, she brings it up on every command, louder each time.
	for level := 1; level <= 4; level++ {
		out, errOut, code := s.run(nail, getNS...)
		if out != nsTable || code != 0 || !strings.Contains(plain(errOut), "web-1") && level < 4 {
			t.Fatalf("level %d: stdout %q stderr %q code %d", level, out, errOut, code)
		}
		if errOut == "" {
			t.Fatalf("level %d: silent", level)
		}
		typed = append(typed, strings.Join(getNS, " "))
	}

	calls := len(s.kubectlCalls())
	out, errOut, code = s.run(nail, delWeb...)
	if code != nailExit || out != "" || !strings.Contains(plain(errOut), "nothing ran, exit 75") {
		t.Fatalf("refusal: stdout %q stderr %q code %d", out, errOut, code)
	}
	if len(s.kubectlCalls()) != calls {
		t.Fatalf("kubectl ran on a refusal: %q", s.kubectlCalls())
	}

	out, errOut, code = s.run(nail, delWeb...)
	if code != 7 || out != "pod \"web-1\" deleted\n" || !strings.Contains(plain(errOut), insisted) {
		t.Fatalf("insist: stdout %q stderr %q code %d", out, errOut, code)
	}
	typed = append(typed, strings.Join(delWeb, " "))

	// Sulking, and still waiting: both, on every command.
	for i := 0; i < 3; i++ {
		out, errOut, code = s.run(nail, getPods...)
		said := strings.Split(strings.TrimSuffix(plain(errOut), "\n"), "\n")
		if code != 0 || out != crashTable || len(said) != 2 || !contains(curtLines, said[0]) {
			t.Fatalf("sulking %d: stdout %q stderr %q code %d", i, out, errOut, code)
		}
		typed = append(typed, strings.Join(getPods, " "))
	}

	calls = len(s.kubectlCalls())
	if _, errOut, code := s.run(nail, "угу"); code != 0 || !strings.Contains(plain(errOut), sulkOver) {
		t.Fatalf("aga: stderr %q code %d", errOut, code)
	}
	if len(s.kubectlCalls()) != calls {
		t.Fatal("aga ran kubectl")
	}

	_, errOut, _ = s.acting([]string{healthy}, getPods...)
	if !strings.Contains(plain(errOut), "sorted itself out. Thanks for listening") {
		t.Fatalf("resolution: %q", errOut)
	}
	typed = append(typed, strings.Join(getPods, " "))

	// kubectl ran exactly the commands typed, in order, and nothing else.
	if got := s.kubectlCalls(); strings.Join(got, "|") != strings.Join(typed, "|") {
		t.Errorf("kubectl calls:\n%q\nwant\n%q", got, typed)
	}

	var st State
	b, err := os.ReadFile(s.statePath())
	if err != nil || json.Unmarshal(b, &st) != nil {
		t.Fatalf("state: %v %s", err, b)
	}
	if len(st.Shared) != 0 || st.Hurt != nil || st.Sulking || st.Ignored != 0 {
		t.Errorf("left over: %s", b)
	}
}

// TestNailHealedAndOldEndToEnd: an hour later she is still unheard, and a
// pod that sorted itself out does not change that.
func TestNailHealedAndOldEndToEnd(t *testing.T) {
	s := newSandbox(t, nailKubectl)
	healthy := s.table("healthy", healthyTable)
	nail := []string{"MISOGYNETES=always", "MISOGYNETES_DAY=3", "MISOGYNETES_SEED=0", healthy}
	hourAgo := time.Now().Add(-time.Hour)
	writeState(t, s.statePath(), &State{Installed: hourAgo, BannerShown: true,
		Shared: []Share{{Object: "shop/pod/web-1", Why: "it's Error", At: hourAgo}}, Ignored: 2})

	if _, errOut, code := s.run(nail, delWeb...); code != nailExit || len(s.kubectlCalls()) != 0 {
		t.Fatalf("an hour later the fix ran: code %d stderr %q", code, errOut)
	}
	_, errOut, code := s.run(nail, getPods...)
	if code != 0 || !strings.Contains(plain(errOut), "sorted itself out. Not that you'd notice") ||
		!strings.Contains(plain(errOut), "web-1 in shop") {
		t.Fatalf("healed: code %d stderr %q", code, errOut)
	}
	if _, errOut, _ := s.run(nail, "get", "ns"); !strings.Contains(plain(errOut), "web-1") && !contains(neverMind, strings.TrimSpace(plain(errOut))) {
		t.Fatalf("no reminder after healing: %q", errOut)
	}
	if _, errOut, _ := s.run(nail, "aga"); !strings.Contains(plain(errOut), "sorted itself out. Thanks for listening") {
		t.Fatalf("aga after healing: %q", errOut)
	}
	if got := s.kubectlCalls(); len(got) != 2 {
		t.Errorf("kubectl calls: %q", got)
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
		Hurt:   &Hurt{Command: "scale deploy web", At: now}, Sulking: true, Ignored: 5})
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
