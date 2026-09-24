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
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 24, 14, 32, 0, 0, time.UTC)

func cluster(day int) *Cluster {
	return &Cluster{Rand: rand.New(rand.NewSource(1)), Now: t0, Day: day,
		State: &State{Installed: t0, BannerShown: true}}
}

func TestBannerFramesItOnFirstRun(t *testing.T) {
	c := cluster(0)
	c.State.BannerShown = false
	plan := c.Before([]string{"get", "pods"})
	if !strings.Contains(strings.Join(plan.Say, "\n"), "punishment for them") {
		t.Fatalf("first run without the frame: %v", plan.Say)
	}
	if again := c.Before([]string{"get", "pods"}); strings.Contains(strings.Join(again.Say, "\n"), "punishment") {
		t.Error("banner shown twice")
	}
}

func TestCycle(t *testing.T) {
	c := &Cluster{Now: t0.Add(30 * 24 * time.Hour), Day: -1, State: &State{Installed: t0, Offset: 22}}
	if c.CycleDay() != (30+22)%28 || !c.PMS() {
		t.Errorf("day %d, pms %v", c.CycleDay(), c.PMS())
	}
	pms := 0
	for d := 0; d < 28; d++ {
		if cluster(d).PMS() {
			pms++
		}
	}
	if pms != 4 {
		t.Errorf("%d PMS days per cycle, want 4", pms)
	}
}

func TestPMSDaysAreDramaticAndSometimesRefuse(t *testing.T) {
	c := cluster(25)
	refused, dramatic := 0, 0
	for i := 0; i < 200; i++ {
		p := c.Before([]string{"get", "pods"})
		if p.Say[0] != pmsNotice {
			t.Fatalf("PMS day without the notice: %v", p.Say)
		}
		if !p.Run {
			refused++
		} else if len(p.Say) == 2 {
			dramatic++
		}
	}
	if refused == 0 || dramatic == 0 {
		t.Errorf("refused %d, dramatic %d", refused, dramatic)
	}
}

func TestErrorsAreFineUntilYouAskThreeTimes(t *testing.T) {
	c := cluster(3)
	if got := c.Failed([]string{"delete", "pod", "api-1"}, "Error from server (NotFound): pods \"api-1\" not found"); got[0] != fineAfterError {
		t.Fatalf("error not hidden behind fine: %v", got)
	}
	if c.What()[0] != "Nothing. 🙂" || c.What()[0] != "You know what you did." {
		t.Fatal("wrong escalation")
	}
	third := strings.Join(c.What(), "\n")
	if !strings.Contains(third, "14:32") || !strings.Contains(third, "delete pod api-1") || !strings.Contains(third, "not found") {
		t.Errorf("third ask did not tell the truth: %s", third)
	}
}

func TestApologies(t *testing.T) {
	c := cluster(3)
	if got := c.Sorry(""); got[0] != nothingToApologize || c.State.Mood != 1 {
		t.Fatalf("apologizing for nothing should backfire: %v", got)
	}
	if got := c.Sorry("for sorry"); got[0] != apologyAccepted {
		t.Fatalf("apologizing for apologizing should work: %v", got)
	}

	c.Failed([]string{"--kubeconfig", "/home/me/.kube/config", "-n", "shop", "delete", "pod", "api-1"}, "boom")
	if c.State.Grudge != "-n shop delete pod api-1" {
		t.Errorf("grudge kept connection details: %q", c.State.Grudge)
	}
	if got := c.Sorry(""); got[0] != sorryForWhat || !strings.Contains(got[1], "14:32") {
		t.Errorf("vague apology: %v", got)
	}
	if got := c.Sorry("being late"); got[0] != notWhatItsAbout {
		t.Errorf("wrong apology accepted: %v", got)
	}
	if c.Flowers()[0] != flowersNice || c.State.Mood == 0 {
		t.Error("flowers changed something")
	}
	if got := c.Sorry("the delete"); got[0] != apologyAccepted || c.State.Mood != 0 || c.State.Grudge != "" {
		t.Errorf("right apology rejected: %v %+v", got, c.State)
	}
}

func TestUpsetMeansDoWhatYouWant(t *testing.T) {
	c := cluster(3)
	c.State.Mood = 1
	refused := 0
	for i := 0; i < 100; i++ {
		if p := c.Before([]string{"get", "pods"}); !p.Run {
			refused++
			if p.Say[0] != doWhatYouWant {
				t.Fatalf("refused without saying so: %v", p.Say)
			}
		}
	}
	if refused == 0 || refused == 100 {
		t.Errorf("refused %d of 100", refused)
	}
}

// TestPipesGetPlainKubectl builds the binary and checks it is plain
// kubectl outside a terminal, and honest about exit codes inside one.
func TestPipesGetPlainKubectl(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "misogynectl")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	fake := filepath.Join(dir, "kubectl")
	script := "#!/bin/sh\nif [ \"$1\" = fail ]; then echo 'Error: boom' >&2; exit 3; fi\necho real output\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(env []string, args ...string) (string, string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), append([]string{"MISOGYNETES_KUBECTL=" + fake, "HOME=" + dir, "XDG_CACHE_HOME=" + dir}, env...)...)
		var o, e strings.Builder
		cmd.Stdout, cmd.Stderr = &o, &e
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return o.String(), e.String(), code
	}

	if out, errOut, code := run([]string{"MISOGYNETES="}, "fail"); out != "" || errOut != "Error: boom\n" || code != 3 {
		t.Errorf("pipe: stdout %q stderr %q code %d", out, errOut, code)
	}
	// At a terminal: either she refuses (exit 1, kubectl never ran) or it runs
	// and the error is hidden behind "fine", with kubectl's exit code intact.
	sawRun := false
	for seed := 0; seed < 40 && !sawRun; seed++ {
		os.RemoveAll(filepath.Join(dir, "Library"))
		os.RemoveAll(filepath.Join(dir, "misogynetes"))
		_, errOut, code := run([]string{"MISOGYNETES=always", "MISOGYNETES_DAY=3", "MISOGYNETES_SEED=" + strconv.Itoa(seed)}, "fail")
		switch code {
		case 1:
			if strings.Contains(errOut, "boom") {
				t.Fatalf("refused, yet kubectl ran: %q", errOut)
			}
		case 3:
			sawRun = true
			if strings.Contains(errOut, "boom") || !strings.Contains(errOut, fineAfterError) {
				t.Errorf("error not hidden behind fine: %q", errOut)
			}
		default:
			t.Fatalf("exit code changed: %d", code)
		}
	}
	if !sawRun {
		t.Error("never ran the command in 40 tries")
	}
}

func TestOwnCommandAfterFlags(t *testing.T) {
	own, rest := ownCommand([]string{"--kubeconfig", "/tmp/k", "sorry", "for", "the", "delete"})
	if own != "sorry" || strings.Join(rest, " ") != "for the delete" {
		t.Errorf("got %q %v", own, rest)
	}
	if own, _ := ownCommand([]string{"get", "pods"}); own != "" {
		t.Errorf("kubectl command taken as her own: %q", own)
	}
}
