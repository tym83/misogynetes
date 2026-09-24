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
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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
	if c.State.Grudge != "delete pod api-1" {
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

func TestOwnCommandOnlyAsFirstWord(t *testing.T) {
	own, rest := ownCommand([]string{"sorry", "for", "the", "delete"})
	if own != "sorry" || strings.Join(rest, " ") != "for the delete" {
		t.Errorf("got %q %v", own, rest)
	}
	for _, args := range [][]string{
		{"get", "pods"},
		{"--as", "what", "get", "pods"},
		{"--user", "flowers", "get", "pods"},
		{"--cluster", "sorry", "delete", "pod", "x"},
		{"--kubeconfig", "/tmp/k", "sorry"},
	} {
		if own, _ := ownCommand(args); own != "" {
			t.Errorf("%v taken as her own %q", args, own)
		}
	}
}

func TestVerbSkipsFlagValues(t *testing.T) {
	for _, tc := range []struct {
		args []string
		verb string
	}{
		{[]string{"--as", "what", "get", "pods"}, "get"},
		{[]string{"--user", "flowers", "get", "pods"}, "get"},
		{[]string{"--cluster", "sorry", "delete", "pod", "x"}, "delete"},
		{[]string{"-v", "6", "get", "pods"}, "get"},
		{[]string{"--as=what", "get", "pods"}, "get"},
		{[]string{"-v=6", "--token=abc", "--insecure-skip-tls-verify", "get"}, "get"},
		{[]string{"--request-timeout", "5s", "--as-group", "admins", "top", "nodes"}, "top"},
		{[]string{"--kubeconfig"}, "nothing"},
	} {
		if got := Verb(tc.args); got != tc.verb {
			t.Errorf("Verb(%v) = %q, want %q", tc.args, got, tc.verb)
		}
	}
}

func TestGrudgeKeepsNoSecrets(t *testing.T) {
	const jwt = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhZG1pbiJ9.c2lnbmF0dXJl"
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--token=" + jwt, "get", "secrets"}, "get secrets"},
		{[]string{"--token", jwt, "get", "secrets"}, "get secrets"},
		{[]string{"get", "pods", "--password=hunter2", "--username", "admin"}, "get pods"},
		{[]string{"--server=https://10.0.0.1:6443", "--kubeconfig", "/root/.kube/admin", "--as", "system:admin", "delete", "pod", "x"}, "delete pod x"},
		{[]string{"--client-key", "/k.pem", "--client-certificate=/c.pem", "-n", "shop", "delete", "pod", "api-1"}, "delete pod api-1"},
		{[]string{"create", "secret", "generic", "db", "--from-literal", "password=hunter2", "--from-file=key=/k"}, "create secret generic"},
		{[]string{"create", "secret", "generic", "--from-env-file", "/e.env", "db"}, "create secret generic"},
		{[]string{"apply", jwt, "abcdef.0123456789abcdef"}, "apply"},
	} {
		c := cluster(3)
		c.Failed(tc.args, "boom")
		if c.State.Grudge != tc.want {
			t.Errorf("%v: grudge %q, want %q", tc.args, c.State.Grudge, tc.want)
		}
		for _, secret := range []string{jwt, "hunter2", "admin/", "/root", "10.0.0.1", "/k.pem", "/c.pem", "/e.env", "abcdef."} {
			if strings.Contains(c.State.Grudge, secret) {
				t.Errorf("%v: grudge %q keeps %q", tc.args, c.State.Grudge, secret)
			}
		}
	}
}

func TestLastErrorIsShortAndRedacted(t *testing.T) {
	const jwt = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhZG1pbiJ9.c2lnbmF0dXJl"
	stderr := "Error: Authorization: Bearer " + jwt + "\n" +
		"error: --token=sekrit1 rejected, token: sekrit2, password=sekrit3\n" +
		"bootstrap abcdef.0123456789abcdef\n" + strings.Repeat("ю", 8<<20)
	c := cluster(3)
	c.Failed([]string{"get", "pods"}, stderr)
	got := c.State.LastError
	if len(got) > maxLastError+16 || !utf8.ValidString(got) {
		t.Errorf("LastError is %d bytes, valid UTF-8 %v", len(got), utf8.ValidString(got))
	}
	for _, secret := range []string{jwt, "eyJ", "sekrit1", "sekrit2", "sekrit3", "0123456789abcdef"} {
		if strings.Contains(got, secret) {
			t.Errorf("LastError keeps %q: %q", secret, got[:200])
		}
	}
	if !strings.HasPrefix(got, "Error: Authorization: Bearer <redacted>") {
		t.Errorf("error lost its shape: %q", got[:120])
	}
}
