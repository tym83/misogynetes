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
	"strings"
	"testing"
	"time"
)

// sightings renders what she noticed, one object per line, for comparison.
func sightings(args []string, out string) string {
	var b strings.Builder
	for _, s := range Notice(args, out, t0) {
		switch {
		case s.Trouble:
			fmt.Fprintf(&b, "trouble %s: %s\n", s.Object, s.Why)
		case s.Healthy:
			fmt.Fprintf(&b, "healthy %s\n", s.Object)
		}
	}
	return b.String()
}

func TestNoticeGetPods(t *testing.T) {
	const out = `NAME          READY   STATUS                  RESTARTS        AGE
crash-1       0/1     CrashLoopBackOff        6 (40s ago)     9m
flappy-1      1/1     Running                 3 (2m ago)      9m
calm-1        1/1     Running                 3 (2d ago)      9d
init-1        0/1     Init:Error              0               1m
pull-1        0/1     ImagePullBackOff        0               1m
oom-1         0/1     OOMKilled               1               1m
pending-1     0/1     Pending                 0               1m
half-1        1/2     Running                 0               1m
job-1         0/1     Completed               0               1h
`
	want := `trouble shop/pod/crash-1: it's CrashLoopBackOff
trouble shop/pod/flappy-1: it keeps restarting
healthy shop/pod/calm-1
trouble shop/pod/init-1: it's Error
trouble shop/pod/pull-1: it's ImagePullBackOff
trouble shop/pod/oom-1: it's OOMKilled
healthy shop/pod/job-1
`
	if got := sightings([]string{"get", "pods", "-n", "shop"}, out); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestNoticeTablesAcrossNamespacesAndKinds(t *testing.T) {
	const all = `NAMESPACE   NAME      READY   STATUS    RESTARTS   AGE
shop        api-1     1/1     Running   0          1d
kube-sys    dns-1     0/1     Error     0          1d
`
	if got, want := sightings([]string{"get", "pods", "-A"}, all), "healthy shop/pod/api-1\ntrouble kube-sys/pod/dns-1: it's Error\n"; got != want {
		t.Errorf("-A: got\n%s", got)
	}

	const getAll = `NAME        READY   STATUS             RESTARTS   AGE
pod/web-1   0/1     CrashLoopBackOff   0          1m

NAME                  READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/web   1/1     1            1           1d

NAME          TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)   AGE
service/web   ClusterIP   10.0.0.1     <none>        80/TCP    1d
`
	if got, want := sightings([]string{"-n", "shop", "get", "all"}, getAll), "trouble shop/pod/web-1: it's CrashLoopBackOff\nhealthy shop/deployment/web\n"; got != want {
		t.Errorf("get all: got\n%s", got)
	}

	const nodes = `NAME     STATUS                        ROLES           AGE   VERSION
node-1   Ready                         control-plane   9d    v1.34.1
node-2   NotReady,SchedulingDisabled   <none>          9d    v1.34.1
node-3   Ready,SchedulingDisabled      <none>          9d    v1.34.1
`
	if got, want := sightings([]string{"get", "no"}, nodes), "healthy /node/node-1\ntrouble /node/node-2: it's NotReady\nhealthy /node/node-3\n"; got != want {
		t.Errorf("nodes: got\n%s", got)
	}
}

func TestNoticeEvents(t *testing.T) {
	const events = `LAST SEEN   TYPE      REASON             OBJECT                MESSAGE
2m          Normal    Scheduled          pod/web-1             Successfully assigned shop/web-1 to node-1
40s         Warning   BackOff            pod/web-1             Back-off restarting failed container web
10s         Warning   Failed Something   pod/odd-1             token: eyJhbGciOiJSUzI1NiJ9.x.y
5s          Warning   FailedScheduling   deployment.apps/api   0/3 nodes are available
`
	want := `trouble shop/pod/web-1: it keeps saying BackOff
trouble shop/pod/odd-1: it keeps saying Warning
trouble shop/deployment/api: it keeps saying FailedScheduling
`
	for _, args := range [][]string{{"events", "-n", "shop"}, {"get", "events", "-n", "shop"}} {
		got := sightings(args, events)
		if got != want {
			t.Errorf("%v: got\n%s", args, got)
		}
		if strings.Contains(got, "eyJ") || strings.Contains(got, "Back-off") {
			t.Errorf("%v: an event message got through: %s", args, got)
		}
	}
}

func TestNoticeDescribe(t *testing.T) {
	const crashing = `Name:             web-1
Namespace:        shop
Status:           Running
Containers:
  web:
    State:          Waiting
      Reason:       CrashLoopBackOff
    Last State:     Terminated
      Reason:       Error
      Exit Code:    1
    Ready:          False
Events:
  Type     Reason   Age   From     Message
  ----     ------   ----  ----     -------
  Warning  BackOff  10s   kubelet  Back-off restarting failed container
`
	// Healthy now: the old OOMKilled is only its last state, the old
	// warnings are only history.
	const recovered = `Name:             web-1
Namespace:        shop
Status:           Running
Containers:
  web:
    State:          Running
    Last State:     Terminated
      Reason:       OOMKilled
    Ready:          True
Events:
  Type     Reason   Age   From     Message
  Warning  BackOff  50m   kubelet  Back-off restarting failed container
`
	const deploy = `Name:                   api
Namespace:              shop
Replicas:               3 desired | 3 updated | 3 total | 2 available | 1 unavailable
Events:
  Type     Reason             Age  From                   Message
  Warning  ReplicaSetFailed   1m   deployment-controller  boom
`
	const evicted = `Name:           batch-1
Namespace:      jobs
Status:         Failed
Reason:         Evicted
`
	for _, tc := range []struct {
		args       []string
		out, sight string
	}{
		{[]string{"describe", "pod", "web-1", "-n", "shop"}, crashing, "trouble shop/pod/web-1: it's CrashLoopBackOff\n"},
		{[]string{"describe", "pod/web-1"}, recovered, "healthy shop/pod/web-1\n"},
		{[]string{"describe", "deploy", "api"}, deploy, "trouble shop/deployment/api: it keeps saying ReplicaSetFailed\n"},
		{[]string{"describe", "pods"}, evicted + "\n" + recovered, "trouble jobs/pod/batch-1: it's Failed\nhealthy shop/pod/web-1\n"},
	} {
		if got := sightings(tc.args, tc.out); got != tc.sight {
			t.Errorf("%v: got\n%s\nwant\n%s", tc.args, got, tc.sight)
		}
	}
}

func TestNoticeLogs(t *testing.T) {
	const bad = "starting\nERROR: connection refused\n"
	for _, tc := range []struct {
		args       []string
		out, sight string
	}{
		{[]string{"logs", "web-1", "-n", "shop"}, bad, "trouble shop/pod/web-1: its logs are full of errors\n"},
		{[]string{"logs", "deploy/api"}, "panic: nil map\n", "trouble /deployment/api: its logs are full of errors\n"},
		{[]string{"logs", "web-1"}, "all good, no errors here\n", ""},
		{[]string{"logs", "-l", "app=web"}, bad, ""},
	} {
		if got := sightings(tc.args, tc.out); got != tc.sight {
			t.Errorf("%v: got %q, want %q", tc.args, got, tc.sight)
		}
	}
}

func TestNoticeKeepsNoSecrets(t *testing.T) {
	const jwt = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhZG1pbiJ9.c2lnbmF0dXJl"
	out := "NAME     READY   STATUS   RESTARTS   AGE\n" +
		jwt + "   0/1     Error    0          1m\n" +
		"Web_1    0/1     Error    0          1m\n"
	if got := sightings([]string{"get", "pods", "--token", jwt}, out); got != "" {
		t.Errorf("kept a secret or an impossible name: %q", got)
	}
	if got := sightings([]string{"get", "pods", "-n", "a=b"}, "NAME  READY  STATUS  RESTARTS  AGE\nweb   0/1    Error   0         1m\n"); got != "" {
		t.Errorf("kept an impossible namespace: %q", got)
	}
}

func TestListensOnlyToReadTables(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		listens bool
	}{
		{[]string{"get", "pods"}, true},
		{[]string{"get", "pods", "-o", "wide"}, true},
		{[]string{"get", "pods", "-owide"}, true},
		{[]string{"get", "pods", "-w"}, true},
		{[]string{"describe", "pod", "x"}, true},
		{[]string{"logs", "-f", "x"}, true},
		{[]string{"events"}, true},
		{[]string{"get", "pods", "-o", "yaml"}, false},
		{[]string{"get", "pods", "--output=json"}, false},
		{[]string{"get", "pods", "-oname"}, false},
		{[]string{"get", "pods", "-o"}, false},
		{[]string{"delete", "pod", "x"}, false},
		{[]string{"exec", "x", "--", "get"}, false},
		{[]string{"logs", "-i", "x"}, false},
		{[]string{"top", "pods"}, false},
		{nil, false},
	} {
		if got := listens(tc.args); got != tc.listens {
			t.Errorf("listens(%v) = %v", tc.args, got)
		}
	}
}

func TestParseAge(t *testing.T) {
	for _, tc := range []struct {
		in  string
		out time.Duration
		ok  bool
	}{
		{"45s", 45 * time.Second, true},
		{"2m30s", 150 * time.Second, true},
		{"3h5m", 3*time.Hour + 5*time.Minute, true},
		{"4d", 96 * time.Hour, true},
		{"", 0, false},
		{"5", 0, false},
		{"m", 0, false},
		{"5x", 0, false},
	} {
		if got, ok := parseAge(tc.in); got != tc.out || ok != tc.ok {
			t.Errorf("parseAge(%q) = %v %v", tc.in, got, ok)
		}
	}
}

func TestSeenNeverFailsAndKeepsTheHead(t *testing.T) {
	s := &seen{}
	chunk := []byte(strings.Repeat("x", 100<<10))
	for i := 0; i < 5; i++ {
		if n, err := s.Write(chunk); n != len(chunk) || err != nil {
			t.Fatalf("write %d: %d %v", i, n, err)
		}
	}
	if len(s.String()) != maxSeen {
		t.Errorf("kept %d bytes", len(s.String()))
	}
}
