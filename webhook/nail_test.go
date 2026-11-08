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

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

const (
	crashing = `{"metadata":{"name":"web-1"},"status":{"phase":"Running","startTime":"2026-11-08T11:00:00Z",
"conditions":[{"type":"Ready","status":"False","lastTransitionTime":"2026-11-08T11:30:00Z"}],
"containerStatuses":[{"state":{"waiting":{"reason":"CrashLoopBackOff","message":"secret stuff"}}}]}}`
	healthy = `{"metadata":{"name":"db-0"},"status":{"phase":"Running",
"conditions":[{"type":"Ready","status":"True","lastTransitionTime":"2026-11-08T10:00:00Z"}],
"containerStatuses":[{"state":{"running":{}}}]}}`
	oom = `{"metadata":{"name":"api-1"},"status":{"phase":"Running",
"conditions":[{"type":"Ready","status":"False","lastTransitionTime":"2026-11-08T11:50:00Z"}],
"containerStatuses":[{"state":{"terminated":{"reason":"OOMKilled"}}}]}}`
	evicted   = `{"metadata":{"name":"old-1"},"status":{"phase":"Failed","reason":"Evicted"}}`
	completed = `{"metadata":{"name":"job-1"},"status":{"phase":"Succeeded","containerStatuses":[{"state":{"terminated":{"reason":"Completed"}}}]}}`
	leaving   = `{"metadata":{"name":"bye-1","deletionTimestamp":"2026-11-08T11:59:00Z"},"status":{"phase":"Running",
"containerStatuses":[{"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}}`
)

func pods(t *testing.T, raw ...string) []Pod {
	t.Helper()
	var out []Pod
	for _, r := range raw {
		var p Pod
		if err := json.Unmarshal([]byte(r), &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

type fakeCluster struct {
	pods  []Pod
	heard time.Time
	err   error
	reads int
}

func (f *fakeCluster) Pods(context.Context, string) ([]Pod, error) {
	f.reads++
	return f.pods, f.err
}

func (f *fakeCluster) HeardAt(context.Context, string) (time.Time, error) { return f.heard, nil }

func TestGrievances(t *testing.T) {
	got := Grievances(pods(t, crashing, healthy, oom, evicted, completed, leaving))
	var lines []string
	for _, g := range got {
		lines = append(lines, g.Object+" "+g.Why+" "+g.Since.Format(time.Kitchen))
	}
	want := "pod/api-1 it's OOMKilled 11:50AM|pod/old-1 it's Evicted 12:00AM|pod/web-1 it's CrashLoopBackOff 11:30AM"
	if strings.Join(lines, "|") != want {
		t.Errorf("grievances:\n%s\nwant\n%s", strings.Join(lines, "|"), want)
	}
}

func request(op, user string) *Request {
	return &Request{
		UID: "1", Kind: GroupKind{Group: "apps", Kind: "Deployment"},
		Resource: GroupResource{Group: "apps", Resource: "deployments"},
		Name:     "web", Namespace: "shop", Operation: op,
		UserInfo: UserInfo{Username: user},
		Object:   json.RawMessage(`{"metadata":{"name":"web"}}`),
	}
}

func misogynist(c *fakeCluster) *Misogynist {
	m := NewMisogynist(c, 5*time.Minute, 1)
	m.Now = func() time.Time { return now }
	return m
}

// In warn mode she shares and never refuses.
func TestWarnSharesAndNeverRefuses(t *testing.T) {
	m := misogynist(&fakeCluster{pods: pods(t, crashing, oom)})
	for _, op := range []string{"CREATE", "UPDATE", "DELETE"} {
		m.Limiter = &Limiter{Window: 5 * time.Minute}
		r := m.Decide(Warn, request(op, "alice"))
		said := strings.Join(r.Warnings, "\n")
		if !r.Allowed || r.Result != nil || !strings.Contains(said, "pod/api-1") || !strings.Contains(said, "And 1 more") ||
			!strings.Contains(said, "misogynetes.dev/heard-at") || strings.Contains(said, "secret stuff") {
			t.Errorf("%s: %+v", op, r)
		}
	}
}

func TestEnforceRefusesUntilHeard(t *testing.T) {
	c := &fakeCluster{pods: pods(t, crashing)}
	m := misogynist(c)
	r := m.Decide(Enforce, request("UPDATE", "alice"))
	if r.Allowed || r.Result == nil || r.Result.Code != http.StatusForbidden ||
		!strings.Contains(r.Result.Message, "pod/web-1") || !strings.Contains(r.Result.Message, "kubectl annotate namespace shop misogynetes.dev/heard-at=") {
		t.Fatalf("not refused: %+v", r)
	}
	// Refusals are never rate-limited: the same fix is refused again.
	if r := m.Decide(Enforce, request("UPDATE", "alice")); r.Allowed {
		t.Fatal("second try went through")
	}
	// Heard after the pod stopped being ready: nothing to say.
	c.heard = now.Add(-time.Minute)
	m = misogynist(c)
	if r := m.Decide(Enforce, request("UPDATE", "alice")); !r.Allowed || len(r.Warnings) != 0 {
		t.Errorf("heard, still refused: %+v", r)
	}
	// Heard before it: still unheard.
	c.heard = now.Add(-2 * time.Hour)
	m = misogynist(c)
	if r := m.Decide(Enforce, request("UPDATE", "alice")); r.Allowed {
		t.Error("heard before the trouble started counts as heard")
	}
}

func TestBreakGlass(t *testing.T) {
	m := misogynist(&fakeCluster{pods: pods(t, crashing)})
	up := request("UPDATE", "alice")
	up.Object = json.RawMessage(`{"metadata":{"annotations":{"misogynetes.dev/break-glass":"INC-42"}}}`)
	if r := m.Decide(Enforce, up); !r.Allowed || !strings.Contains(strings.Join(r.Warnings, " "), "INC-42") {
		t.Errorf("break-glass update: %+v", r)
	}
	del := request("DELETE", "alice")
	del.Object = nil
	del.OldObject = json.RawMessage(`{"metadata":{"annotations":{"misogynetes.dev/break-glass":"INC-42"}}}`)
	if r := m.Decide(Enforce, del); !r.Allowed {
		t.Errorf("break-glass delete: %+v", r)
	}
	// An old annotation does not let an update through that removes it.
	up.OldObject, up.Object = up.Object, json.RawMessage(`{"metadata":{}}`)
	if r := m.Decide(Enforce, up); r.Allowed {
		t.Error("break-glass from the old object let an update through")
	}
}

// Nodes, controllers, service accounts, system namespaces, leases and
// events pass untouched, refused never, in either mode.
func TestPassThrough(t *testing.T) {
	c := &fakeCluster{pods: pods(t, crashing)}
	m := misogynist(c)
	reqs := []*Request{}
	for _, user := range []string{"system:node:node-1", "system:kube-scheduler", "system:serviceaccount:argocd:argocd-application-controller",
		"system:serviceaccount:kube-system:deployment-controller", ""} {
		reqs = append(reqs, request("UPDATE", user))
	}
	for _, res := range []GroupResource{{Resource: "nodes"}, {Group: "coordination.k8s.io", Resource: "leases"}, {Resource: "events"}, {Resource: "secrets"}} {
		r := request("UPDATE", "alice")
		r.Resource = res
		reqs = append(reqs, r)
	}
	sys := request("UPDATE", "alice")
	sys.Namespace = "kube-system"
	reqs = append(reqs, sys)
	for _, r := range reqs {
		for _, mode := range []Mode{Warn, Enforce} {
			if got := m.Decide(mode, r); !got.Allowed || len(got.Warnings) != 0 {
				t.Errorf("%s %+v: %+v", mode, r, got)
			}
		}
	}
	if c.reads != 0 {
		t.Errorf("read the cluster %d times for requests she passes", c.reads)
	}
	m.ServiceAccounts = true
	if got := m.Decide(Enforce, request("UPDATE", "system:serviceaccount:argocd:argocd-application-controller")); got.Allowed {
		t.Error("service accounts on: argocd not refused in enforce mode")
	}
	m.Off = true
	if got := m.Decide(Enforce, request("UPDATE", "alice")); !got.Allowed || len(got.Warnings) != 0 {
		t.Errorf("off: %+v", got)
	}
}

// When she cannot read the cluster she says nothing and refuses nothing.
func TestBlindMeansAllowed(t *testing.T) {
	m := misogynist(&fakeCluster{err: errors.New("forbidden")})
	if r := m.Decide(Enforce, request("UPDATE", "alice")); !r.Allowed || len(r.Warnings) != 0 {
		t.Errorf("blind: %+v", r)
	}
}

func TestWarnRateLimit(t *testing.T) {
	c := &fakeCluster{pods: pods(t, crashing)}
	m := misogynist(c)
	if len(m.Decide(Warn, request("UPDATE", "alice")).Warnings) == 0 {
		t.Fatal("first: silent")
	}
	if w := m.Decide(Warn, request("CREATE", "alice")).Warnings; len(w) != 0 {
		t.Errorf("again within the window: %q", w)
	}
	if len(m.Decide(Warn, request("UPDATE", "bob")).Warnings) == 0 {
		t.Error("another person: silent")
	}
	if c.reads != 1 {
		t.Errorf("read the cluster %d times in one burst", c.reads)
	}
}

func TestHandler(t *testing.T) {
	m := misogynist(&fakeCluster{pods: pods(t, crashing)})
	body, _ := json.Marshal(Review{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview", Request: request("UPDATE", "alice")})
	rec := httptest.NewRecorder()
	Handler(m.In(Enforce)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/enforce", bytes.NewReader(body)))
	var out Review
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Response == nil {
		t.Fatalf("%q: %v", rec.Body.String(), err)
	}
	if out.Response.UID != "1" || out.Response.Allowed || out.Response.Result == nil {
		t.Errorf("enforce: %+v", out.Response)
	}
}

func TestAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("she wrote: %s %s", r.Method, r.URL)
		}
		switch r.URL.Path {
		case "/api/v1/namespaces/shop/pods":
			w.Write([]byte(`{"items":[` + crashing + `]}`))
		case "/api/v1/namespaces/shop":
			w.Write([]byte(`{"metadata":{"annotations":{"misogynetes.dev/heard-at":"2026-11-08T11:45:00Z"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	a := &API{Host: srv.URL, Client: srv.Client()}
	ps, err := a.Pods(context.Background(), "shop")
	if err != nil || len(ps) != 1 || ps[0].Metadata.Name != "web-1" {
		t.Errorf("pods: %+v %v", ps, err)
	}
	heard, err := a.HeardAt(context.Background(), "shop")
	if err != nil || !heard.Equal(now.Add(-15*time.Minute)) {
		t.Errorf("heard: %v %v", heard, err)
	}
	if _, err := a.Pods(context.Background(), "nope/../x"); err == nil {
		t.Error("an unknown path did not fail")
	}
}
