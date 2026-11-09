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
	for _, later := range []time.Duration{0, 20 * time.Minute, 5 * time.Hour} {
		c.Now = t0.Add(later)
		if again := c.Observe(getPods, crashTable); len(again) != 0 {
			t.Errorf("after %v: announced the same nail again: %q", later, again)
		}
	}
	// Heard, the same trouble is one grievance for as long as it lasts: a
	// crash loop that goes on is not news, however long.
	c.Now = t0
	c.Aga()
	for _, later := range []time.Duration{0, 20 * time.Minute, 5 * time.Hour} {
		c.Now = t0.Add(later)
		if again := c.Observe(getPods, crashTable); len(again) != 0 || c.unheard() {
			t.Errorf("after %v: the same heard trouble announced again: %q", later, again)
		}
	}
	// Up for streak.Window, it sorted itself out; trouble after that is new.
	c.Observe(getPods, healthyTable)
	c.Now = c.Now.Add(time.Hour)
	if again := c.Observe(getPods, crashTable); !contains(again, "web-1") || !c.unheard() {
		t.Errorf("new trouble after health: %q", again)
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

// nailKubectl logs every call and prints the table in $TABLE for get. A
// delete with DELETE_EXIT set fails without deleting anything.
const nailKubectl = `echo "$*" >> "$0.log"
case "$1" in
get) if [ "$2" = pods ]; then cat "$TABLE"; else printf 'NAME      STATUS   AGE\ndefault   Active   9d\n'; fi ;;
delete) if [ "${DELETE_EXIT:-0}" = 0 ]; then echo "pod \"$3\" deleted"; else echo "error: timed out waiting for the condition" >&2; fi; exit "${DELETE_EXIT:-0}" ;;
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
	if code != 7 || out != "" || !strings.Contains(plain(errOut), insisted) {
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
		if out != "" || errOut != "error: timed out waiting for the condition\n" || code != 7 {
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

// Once the pod is gone, so is the nail: she does not keep naming a pod
// that no longer exists. The sulk from being overruled stays until "aga".
func TestDeletedObjectIsNoLongerBroughtUp(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	c.Before(delWeb)
	c.Now = c.Now.Add(time.Second)
	if p := c.Before(delWeb); !p.Run {
		t.Fatalf("insisting: %+v", p)
	}
	c.Gone(deletedObjects(delWeb))
	if c.unheard() || len(c.State.Shared) != 0 || c.State.Ignored != 0 {
		t.Fatalf("still waiting about a deleted pod: %+v", c.State)
	}
	said := remind(t, c, []string{"get", "ns"})
	if len(said) != 1 || !contains(curtLines, said[0]) || contains(said, "web-1") {
		t.Errorf("after the delete: %q", said)
	}
}

func TestDeletedObjects(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{delWeb, "shop/pod/web-1"},
		{[]string{"delete", "pods", "a", "b"}, "/pod/a /pod/b"},
		{[]string{"-n", "shop", "delete", "pod/a", "deploy/web"}, "shop/pod/a shop/deployment/web"},
		{[]string{"delete", "node", "node-2"}, "/node/node-2"},
		{[]string{"delete", "pod", "-l", "app=web"}, ""},
		{[]string{"delete", "-f", "web.yaml"}, ""},
		{[]string{"delete", "pod,svc", "web"}, ""},
		{[]string{"get", "pod", "web-1"}, ""},
	} {
		if got := strings.Join(deletedObjects(tc.args), " "); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestNotFoundIsGone(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	stderr := `Error from server (NotFound): pods "web-1" not found` + "\n"
	if got := strings.Join(notFound(getPods, stderr), " "); got != "shop/pod/web-1" {
		t.Fatalf("notFound: %q", got)
	}
	c.Gone(notFound([]string{"get", "pods", "web-1"}, stderr))
	if c.unheard() {
		t.Errorf("still waiting about a pod that is not found: %+v", c.State.Shared)
	}
}

// She goes by how the pod is now: running and ready, with its last restart
// over streak.Window ago, it sorted itself out, however recent the trouble
// was. A restart within the window is the same crash loop.
func TestSortedItselfOutAsSoonAsItIsHealthy(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, crashTable)
	c.Aga()
	c.Now = c.Now.Add(2 * time.Minute)
	for _, ago := range []string{"30s", "4m0s"} {
		settling := "NAME     READY   STATUS    RESTARTS      AGE\nweb-1    1/1     Running   7 (" + ago + " ago)   11m\n"
		if got := c.Observe(getPods, settling); len(got) != 0 {
			t.Errorf("restarted %s ago, already sorted out: %q", ago, got)
		}
	}
	settled := "NAME     READY   STATUS    RESTARTS     AGE\nweb-1    1/1     Running   7 (6m ago)   17m\n"
	if got := c.Observe(getPods, settled); !contains(got, "pod/web-1 in shop sorted itself out") {
		t.Errorf("healthy now, not sorted out: %q", got)
	}
}

func TestDeleteThatWentThroughEndsTheNail(t *testing.T) {
	s := newSandbox(t, nailKubectl)
	crash := s.table("crash", crashTable)
	nail := []string{"MISOGYNETES=always", "MISOGYNETES_DAY=3", "MISOGYNETES_SEED=0", crash}
	if _, errOut, _ := s.acting([]string{crash}, getPods...); !strings.Contains(plain(errOut), "pod/web-1 in shop") {
		t.Fatalf("share: %q", errOut)
	}
	if _, _, code := s.run(nail, delWeb...); code != nailExit {
		t.Fatalf("refusal: code %d", code)
	}
	if _, errOut, code := s.run(nail, delWeb...); code != 0 || !strings.Contains(plain(errOut), insisted) {
		t.Fatalf("insist: stderr %q code %d", errOut, code)
	}
	for i := 0; i < 3; i++ {
		_, errOut, code := s.run(nail, "get", "ns")
		if code != 0 || strings.Contains(errOut, "web-1") {
			t.Fatalf("command %d after the delete: stderr %q code %d", i, errOut, code)
		}
	}
}

func TestDeletedInOutput(t *testing.T) {
	out := `pod "web-1" deleted
deployment.apps "api" deleted from shop namespace
pod "db-0" deleted (dry run)
service "web" deleted from other namespace
Warning: something
`
	for _, tc := range []struct {
		args []string
		out  string
		want string
	}{
		{[]string{"delete", "pods", "-l", "app=web", "-n", "shop"}, out,
			"shop/pod/web-1 shop/deployment/api other/service/web"},
		{[]string{"delete", "-f", "all.yaml"}, out, "/pod/web-1 shop/deployment/api other/service/web"},
		{[]string{"delete", "-f", "all.yaml", "-o", "name"}, "pod/web-1\ndeployment.apps/api\n", "/pod/web-1 /deployment/api"},
		{[]string{"delete", "-f", "all.yaml"}, "pod/web-1\n", ""},
		{[]string{"get", "pods"}, out, ""},
	} {
		if got := strings.Join(deletedInOutput(tc.args, tc.out), " "); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestOwnedBy(t *testing.T) {
	for _, tc := range []struct {
		object, controller string
		want               bool
	}{
		{"shop/pod/web-6d4f8c9b7-xk2lp", "shop/deployment/web", true},
		{"/pod/web-6d4f8c9b7-xk2lp", "shop/deployment/web", true},
		{"shop/replicaset/web-6d4f8c9b7", "shop/deployment/web", true},
		{"other/pod/web-6d4f8c9b7-xk2lp", "shop/deployment/web", false},
		{"shop/pod/web-api-6d4f8c9b7-xk2lp", "shop/deployment/web", false},
		{"shop/pod/web-1", "shop/deployment/web", false},
		{"shop/pod/web-1", "shop/statefulset/web", true},
		{"shop/pod/web-xk2lp", "shop/daemonset/web", true},
		{"shop/pod/web-xk2lp", "shop/replicaset/web", true},
		{"shop/job/backup-28912345", "shop/cronjob/backup", true},
		{"shop/pod/backup-28912345-xk2lp", "shop/cronjob/backup", true},
		{"shop/pod/web-6d4f8c9b7-xk2lp", "shop/service/web", false},
	} {
		if got := ownedBy(tc.object, tc.controller); got != tc.want {
			t.Errorf("ownedBy(%q, %q) = %v", tc.object, tc.controller, got)
		}
	}
}

const deployCrashTable = `NAME                   READY   STATUS             RESTARTS      AGE
web-6d4f8c9b7-xk2lp    0/1     CrashLoopBackOff   6 (40s ago)   9m
api-7f9c6b8d5-q8wzt    0/1     CrashLoopBackOff   6 (40s ago)   9m
`

// Deleting a Deployment takes its pods with it, and she stops naming them;
// a pod of another Deployment stays on her mind.
func TestDeletedDeploymentTakesItsPods(t *testing.T) {
	c := cluster(3)
	c.Observe(getPods, deployCrashTable)
	if len(c.State.Shared) != 2 {
		t.Fatalf("shared: %+v", c.State.Shared)
	}
	c.GoneWithDependents(deletedInOutput([]string{"delete", "deploy", "web", "-n", "shop"}, `deployment.apps "web" deleted`+"\n"))
	if got := c.waiting(); len(got) != 1 || !strings.Contains(got[0], "api-7f9c6b8d5-q8wzt") {
		t.Errorf("waiting after the delete: %q", got)
	}
}

// deleteKubectl prints the table in $TABLE for get and whatever is in
// $DELETED for delete, the way kubectl reports a selector or file delete.
const deleteKubectl = `case "$1" in
get) if [ "$2" = pods ]; then cat "$TABLE"; else printf 'NAME      STATUS   AGE\ndefault   Active   9d\n'; fi ;;
delete) cat "$DELETED" ;;
esac
`

func TestDeleteBySelectorOrFileEndsTheNail(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		deleted string
	}{
		{"selector", []string{"delete", "pods", "-l", "app=web", "-n", "shop"}, `pod "web-6d4f8c9b7-xk2lp" deleted` + "\n" + `pod "api-7f9c6b8d5-q8wzt" deleted` + "\n"},
		{"file", []string{"delete", "-f", "shop.yaml"}, `deployment.apps "web" deleted from shop namespace` + "\n" + `deployment.apps "api" deleted from shop namespace` + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSandbox(t, deleteKubectl)
			table := s.table("crash", deployCrashTable)
			deleted := filepath.Join(s.dir, "deleted")
			if err := os.WriteFile(deleted, []byte(tc.deleted), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, errOut, _ := s.acting([]string{table}, getPods...); !strings.Contains(plain(errOut), "web-6d4f8c9b7-xk2lp") {
				t.Fatalf("share: %q", errOut)
			}
			env := []string{"MISOGYNETES=always", "MISOGYNETES_DAY=3", "MISOGYNETES_SEED=0", table, "DELETED=" + deleted}
			if _, _, code := s.run(env, tc.args...); code != nailExit {
				t.Fatalf("refusal: code %d", code)
			}
			if out, _, code := s.run(env, tc.args...); code != 0 || out != tc.deleted {
				t.Fatalf("insist: stdout %q code %d", out, code)
			}
			_, errOut, _ := s.run(env, "get", "ns")
			if strings.Contains(errOut, "xk2lp") || strings.Contains(errOut, "q8wzt") {
				t.Errorf("still bringing up deleted pods: %q", errOut)
			}
		})
	}
}
