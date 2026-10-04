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

const (
	readyNodes = `NAME     STATUS   ROLES    AGE   VERSION
node-1   Ready    <none>   9d    v1.34.1
node-2   Ready    <none>   9d    v1.34.1
`
	notReadyNodes = `NAME     STATUS     ROLES    AGE   VERSION
node-1   Ready      <none>   9d    v1.34.1
node-2   NotReady   <none>   9d    v1.34.1
`
)

func TestClassify(t *testing.T) {
	failing := Notice([]string{"get", "pods", "-A"}, `NAMESPACE   NAME    READY   STATUS             RESTARTS   AGE
shop        a-1     0/1     CrashLoopBackOff   0          1m
shop        b-1     0/1     Error              0          1m
shop        c-1     0/1     ImagePullBackOff   0          1m
blog        d-1     0/1     Error              0          1m
blog        e-1     1/1     Running            0          1m
`, t0)
	nodes := Notice([]string{"get", "nodes"}, notReadyNodes, t0)
	events := parseEvents(`NAMESPACE   LAST SEEN   TYPE      REASON        OBJECT          MESSAGE
db          1m          Warning   FailedMount   pod/db-0        MountVolume.SetUp failed
db          1m          Warning   FailedMount   pod/db-1        MountVolume.SetUp failed
db          1m          Warning   FailedMount   pod/db-2        MountVolume.SetUp failed
web         1m          Warning   Failed        ingress/www     x509: certificate has expired
web         1m          Normal    Sync          ingress/www     certificate renewed
`, "")
	pvcs := parsePVCs(`NAMESPACE   NAME     STATUS    VOLUME   CAPACITY   ACCESS MODES   STORAGECLASS   AGE
db          data-0   Lost      pv-1     1Gi        RWO            ssd            9d
db          data-1   Pending                                      ssd            25m
db          data-2   Pending                                      ssd            2m
db          data-3   Bound     pv-3     1Gi        RWO            ssd            9d
`, "")
	deploys := parseDeploys(`{"items":[
 {"metadata":{"name":"api","namespace":"shop","creationTimestamp":"2026-09-24T10:00:00Z"},"spec":{"replicas":3},"status":{"availableReplicas":0}},
 {"metadata":{"name":"new","namespace":"shop","creationTimestamp":"2026-09-24T14:31:30Z"},"spec":{"replicas":3},"status":{}},
 {"metadata":{"name":"zero","namespace":"shop","creationTimestamp":"2026-09-24T10:00:00Z"},"spec":{"replicas":0},"status":{}},
 {"metadata":{"name":"ok","namespace":"shop","creationTimestamp":"2026-09-24T10:00:00Z"},"spec":{"replicas":2},"status":{"availableReplicas":2}}]}`, true, t0)

	got := Classify(Snapshot{Pods: failing, Nodes: nodes, Events: events, PVCs: pvcs, Deploys: deploys})
	var b strings.Builder
	for _, it := range got {
		fmt.Fprintf(&b, "%s: %s\n", it.Object, it.Why)
	}
	want := `/node/node-2: node NotReady
shop/pod/a-1: CrashLoopBackOff (one of 3 failing pods in shop)
shop/pod/b-1: Error (one of 3 failing pods in shop)
shop/pod/c-1: ImagePullBackOff (one of 3 failing pods in shop)
db/pod/db-0: FailedMount (volume storm: 3 warnings)
db/pod/db-1: FailedMount (volume storm: 3 warnings)
db/pod/db-2: FailedMount (volume storm: 3 warnings)
web/ingress/www: Failed (a certificate problem)
db/persistentvolumeclaim/data-0: volume claim Lost
db/persistentvolumeclaim/data-1: volume claim Pending for 25m0s
shop/deployment/api: 0/3 replicas available
`
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
	if strings.Contains(fmt.Sprint(events), "x509") || strings.Contains(fmt.Sprint(events), "MountVolume") {
		t.Error("kept an event message")
	}

	// Two failing pods, one OOMKilled, healthy nodes: nothing serious.
	if got := Classify(Snapshot{Pods: failing[3:], Nodes: Notice([]string{"get", "nodes"}, readyNodes, t0)}); len(got) != 0 {
		t.Errorf("not serious: %+v", got)
	}
	oom := Notice([]string{"get", "pods"}, `NAME   READY   STATUS      RESTARTS   AGE
a-1    0/1     OOMKilled   1          1m
b-1    0/1     OOMKilled   1          1m
`, t0)
	if got := Classify(Snapshot{Pods: oom, Events: []EventRow{{Object: "/node/node-1", Reason: "OOMKilling"}}}); len(got) != 3 {
		t.Errorf("OOM storm: %+v", got)
	}
	if got := Classify(Snapshot{APIError: "returning server errors"}); len(got) != 1 || got[0].Object != apiserver {
		t.Errorf("API: %+v", got)
	}
}

func TestTalkHealedBeforeItHappened(t *testing.T) {
	c := cluster(3)
	items := []TalkItem{{Object: "/node/node-2", Why: "node NotReady"}}
	c.updateTalk("prod", items)
	c.Now = t0.Add(time.Minute)
	if got := c.updateTalk("prod", nil); len(got) != 0 {
		t.Fatalf("fine after one clear look: %q", got)
	}
	c.Now = t0.Add(time.Minute + calmFor)
	if got := c.updateTalk("prod", nil); len(got) != 1 || got[0] != forgetIt || c.State.Talk == nil {
		t.Fatalf("healed: %q %+v", got, c.State.Talk)
	}
	if !c.unheard() {
		t.Error("the talk went away before it happened")
	}
	c.What()
	c.What()
	got := strings.Join(c.What(), "\n")
	if !strings.Contains(got, "node/node-2: node NotReady") || !strings.Contains(got, talkAllFine) || c.State.Talk != nil {
		t.Errorf("summary after healing: %s", got)
	}

	c.updateTalk("prod", items)
	c.Aga()
	if c.unheard() || c.State.Talk == nil {
		t.Errorf("aga: %+v", c.State.Talk)
	}
	c.updateTalk("prod", nil)
	c.Now = c.Now.Add(calmFor)
	c.updateTalk("prod", nil)
	if c.State.Talk != nil {
		t.Errorf("heard and healed, still kept: %+v", c.State.Talk)
	}
}

// A crash loop flickers: between restarts its deployment briefly has a
// replica available, and one of three failing pods is mid-restart. A clear
// look in between is not "fine", and the talk does not start over.
func TestTalkFlickerIsNotFine(t *testing.T) {
	c := cluster(3)
	items := []TalkItem{{Object: "shop/deployment/crash", Why: "0/1 replicas available"}}
	if got := c.updateTalk("prod", items); len(got) != 1 {
		t.Fatalf("no talk: %q", got)
	}
	c.Aga()
	for i := 1; i <= 6; i++ {
		c.Now = t0.Add(time.Duration(i) * 30 * time.Second)
		var got []string
		if i%2 == 1 {
			got = c.updateTalk("prod", nil)
		} else {
			got = c.updateTalk("prod", items)
		}
		if len(got) != 0 {
			t.Fatalf("look %d: flicker made her say %q", i, got)
		}
	}
	if c.State.Talk == nil || !c.State.Talk.Done || c.State.Talk.Fine {
		t.Fatalf("talk after flicker: %+v", c.State.Talk)
	}
	c.Now = c.Now.Add(30 * time.Second)
	c.updateTalk("prod", nil)
	c.Now = c.Now.Add(calmFor)
	if got := c.updateTalk("prod", nil); len(got) != 1 || got[0] != forgetIt {
		t.Errorf("calm for a minute: %q", got)
	}
}
