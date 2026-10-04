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
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// suitorDeploy is deployment/web with a crowd in its managedFields, all
// relative to t0 (14:32 UTC): the family, helm, argocd, someone's
// kubectl edit, the user's own apply, and something two days old. A
// secret-looking value sits in a key name where only values could be.
const suitorDeploy = `{"items":[{"metadata":{"name":"web","namespace":"shop","creationTimestamp":"2026-01-01T00:00:00Z",
 "managedFields":[
  {"manager":"kube-controller-manager","operation":"Update","time":"2026-09-24T14:00:00Z","fieldsV1":{"f:metadata":{"f:annotations":{}}}},
  {"manager":"kube-controller-manager","operation":"Update","subresource":"status","time":"2026-09-24T14:00:00Z","fieldsV1":{"f:status":{"f:replicas":{}}}},
  {"manager":"helm","operation":"Update","time":"2026-09-24T03:12:00Z","fieldsV1":{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"web\"}":{"f:env":{"k:{\"name\":\"DB_PASSWORD\",\"value\":\"hunter2\"}":{}}}}}}}}},
  {"manager":"argocd-controller","operation":"Apply","time":"2026-09-24T14:20:00Z","fieldsV1":{"f:metadata":{"f:labels":{"f:app":{}}},"f:spec":{"f:replicas":{}}}},
  {"manager":"kubectl-edit","operation":"Update","time":"2026-09-24T13:30:00Z","fieldsV1":{"f:spec":{"f:replicas":{}}}},
  {"manager":"kubectl-client-side-apply","operation":"Update","time":"2026-09-24T14:30:30Z","fieldsV1":{"f:metadata":{"f:annotations":{"f:kubectl.kubernetes.io/last-applied-configuration":{}}}}},
  {"manager":"old-friend","operation":"Update","time":"2026-09-22T10:00:00Z","fieldsV1":{"f:spec":{"f:paused":{}}}}
 ]},"spec":{"replicas":2},"status":{"availableReplicas":2}}]}`

func suitorCluster(t *testing.T) *Cluster {
	t.Helper()
	c := cluster(3)
	c.Now = t0.Add(-2 * time.Minute)
	c.noteMine([]string{"apply", "-f", "web.yaml"}) // the user's own apply at 14:30
	c.Now = t0
	var changes []FieldEntry
	for _, d := range parseDeploys(suitorDeploy, true, t0) {
		changes = append(changes, d.Fields...)
	}
	c.noteSuitors(changes)
	return c
}

func TestSuitorsAreRealAndNotYours(t *testing.T) {
	c := suitorCluster(t)
	var got strings.Builder
	for _, e := range c.State.Suitors {
		fmt.Fprintf(&got, "%s %s %s %s %s\n", e.Object, e.Manager, e.Operation, e.At.UTC().Format("15:04"), strings.Join(e.Fields, ","))
	}
	want := `shop/deployment/web argocd-controller Apply 14:20 metadata.labels,spec.replicas
shop/deployment/web kubectl-edit Update 13:30 spec.replicas
shop/deployment/web helm Update 03:12 spec.template
`
	if got.String() != want {
		t.Errorf("suitors:\n%s\nwant\n%s", got.String(), want)
	}
	for _, leak := range []string{"hunter2", "DB_PASSWORD", "last-applied", "app", "\"web\""} {
		for _, e := range c.State.Suitors {
			if strings.Contains(strings.Join(e.Fields, " "), leak) {
				t.Errorf("leaked %q: %+v", leak, e)
			}
		}
	}
}

func TestWhoTwice(t *testing.T) {
	c := suitorCluster(t)
	if got := c.Who(); len(got) != 1 || got[0] != justAFriend {
		t.Fatalf("first who: %q", got)
	}
	got := strings.Join(c.Who(), "\n")
	for _, want := range []string{
		whoTruth,
		"deployment/web in shop: argocd-controller (Apply) at " + t0.Add(-12*time.Minute).Local().Format("2006-01-02 15:04:05 MST") + ", metadata.labels, spec.replicas",
		"kubectl-edit (Update) at " + time.Date(2026, 9, 24, 13, 30, 0, 0, time.UTC).Local().Format("2006-01-02 15:04:05 MST"),
		"helm (Update)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("second who without %q:\n%s", want, got)
		}
	}
	for _, not := range []string{"kube-controller-manager", "client-side-apply", "old-friend", "hunter2"} {
		if strings.Contains(got, not) {
			t.Errorf("second who names %q:\n%s", not, got)
		}
	}
	if c.Who()[0] != justAFriend {
		t.Error("the truth did not start over")
	}
	if got := cluster(3).Who(); got[0] != nobody {
		t.Errorf("nobody there: %q", got)
	}
}

func TestJealousyIsOccasionalAndTrue(t *testing.T) {
	c := suitorCluster(t)
	said := 0
	var lines []string
	for i := 0; i < 400; i++ {
		c.Now = t0.Add(time.Duration(i) * time.Minute)
		if l := c.jealous([]string{"get", "pods"}); l != "" {
			said++
			lines = append(lines, l)
		}
	}
	if said == 0 || said > 400/30+1 {
		t.Errorf("%d remarks in 400 minutes", said)
	}
	for _, l := range lines {
		ok := false
		for _, who := range []string{"argocd-controller", "kubectl edit", "helm"} {
			ok = ok || strings.Contains(l, who)
		}
		if !ok || !strings.Contains(l, "deployment/web") {
			t.Errorf("not about a real change: %q", l)
		}
	}

	// The gap: never twice within 30 minutes.
	c = suitorCluster(t)
	var last time.Time
	for i := 0; i < 600; i++ {
		c.Now = t0.Add(time.Duration(i) * 10 * time.Second)
		if c.jealous([]string{"get", "pods"}) != "" {
			if !last.IsZero() && c.Now.Sub(last) < jealousGap {
				t.Fatalf("jealous again after %v", c.Now.Sub(last))
			}
			last = c.Now
		}
	}

	// Nothing recent, nothing to say.
	c = suitorCluster(t)
	c.Now = t0.Add(suitorWindow + time.Hour)
	for i := 0; i < 50; i++ {
		if l := c.jealous([]string{"get", "pods"}); l != "" {
			t.Fatalf("jealous of old news: %q", l)
		}
	}
}

func TestCurrentManagerIsNotASuitor(t *testing.T) {
	c := cluster(3)
	c.State.Suitors = []FieldEntry{{Object: "shop/deployment/web", Manager: "kubectl-edit", Operation: "Update", At: t0, Fields: []string{"spec.replicas"}}}
	if got := c.suitors([]string{"edit", "deploy", "web"}); len(got) != 0 {
		t.Errorf("own manager kept: %+v", got)
	}
	if got := c.suitors([]string{"get", "pods"}); len(got) != 1 {
		t.Errorf("lost: %+v", got)
	}
	if managerOf([]string{"apply", "--server-side", "-f", "x"}) != "kubectl" || managerOf([]string{"apply", "--field-manager=me"}) != "me" {
		t.Error("managerOf")
	}
}

func TestWatcherSeesSuitors(t *testing.T) {
	f := calmCluster()
	f.out["deployments"] = suitorDeploy
	tw := newTestWatcher(t, f)
	tw.at(t, 0)
	var dep []string
	for _, c := range f.calls {
		if Words(c)[1] == "deployments" {
			dep = c
		}
	}
	if !strings.Contains(strings.Join(dep, " "), "--show-managed-fields") || !readOnlyCall(dep) {
		t.Errorf("deployments call: %q", dep)
	}
	if st := tw.state(t); len(st.Suitors) != 4 { // nothing of the user's: no Mine here
		t.Errorf("suitors: %+v", st.Suitors)
	}
}

// TestSuitorsStayQuietInPipes: in a pipe "who" is kubectl's, and she adds
// nothing.
func TestSuitorsStayQuietInPipes(t *testing.T) {
	s := newSandbox(t, echoKubectl)
	at := time.Now().Add(-time.Hour)
	writeState(t, s.statePath(), &State{Installed: at, BannerShown: true,
		Suitors: []FieldEntry{{Object: "shop/deployment/web", Manager: "helm", Operation: "Update", At: at, Fields: []string{"spec.template"}}}})
	for seed := 0; seed < 20; seed++ {
		env := []string{"MISOGYNETES=", "MISOGYNETES_SEED=" + strconv.Itoa(seed)}
		if out, errOut, code := s.run(env, "get", "pods"); out != "kubectl ran: get pods\n" || errOut != "" || code != 0 {
			t.Fatalf("pipe: %q %q %d", out, errOut, code)
		}
	}
	if out, errOut, _ := s.run([]string{"MISOGYNETES=off"}, "who"); out != "kubectl ran: who\n" || errOut != "" {
		t.Errorf("who in a pipe: %q %q", out, errOut)
	}
	// At a terminal she does bring it up, now and then.
	heard := false
	for seed := 0; seed < 40 && !heard; seed++ {
		_, errOut, _ := s.run([]string{"MISOGYNETES=always", "MISOGYNETES_DAY=3", "MISOGYNETES_SEED=" + strconv.Itoa(seed)}, "get", "pods")
		heard = strings.Contains(errOut, "helm")
		if !heard {
			continue
		}
		b, _ := os.ReadFile(s.statePath())
		if !strings.Contains(string(b), "jealousAt") {
			t.Error("the gap is not remembered")
		}
	}
	if !heard {
		t.Error("never jealous at a terminal")
	}
}
