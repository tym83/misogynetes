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
	"regexp"
	"strings"
	"time"
)

// "We need to talk." When something serious is wrong in the cluster she
// says only that, and refuses to say more until the third "what". Then she
// tells the whole truth: what, why, since when, and where to look. The joke
// is the delay, never the facts.

// Talk is the serious conversation she is waiting to have.
type Talk struct {
	Context string     `json:"context"`
	Since   time.Time  `json:"since"`
	Items   []TalkItem `json:"items"`
	Asked   int        `json:"asked,omitempty"`
	Done    bool       `json:"done,omitempty"` // talked it through, or you said "aga"
	Fine    bool       `json:"fine,omitempty"` // everything is healthy again
}

// TalkItem is one serious thing, as accurate as she can make it.
type TalkItem struct {
	Object string    `json:"object"` // namespace/kind/name
	Why    string    `json:"why"`
	Since  time.Time `json:"since"` // when she first saw it
}

const (
	// stormSize is how many failures at once make a storm.
	stormSize = 3
	// pvcPendingFor is how long a claim may stay Pending before it is serious.
	pvcPendingFor = 10 * time.Minute
	// deployGrace is how old a deployment with nothing available must be.
	deployGrace = 2 * time.Minute
)

// apiserver is how the API server is remembered.
const apiserver = "/apiserver/cluster"

// Snapshot is what one round of her own looking found.
type Snapshot struct {
	APIError string // "unreachable", "returning server errors"; empty when fine
	Pods     []Sighting
	Nodes    []Sighting
	Events   []EventRow
	PVCs     []PVCRow
	Deploys  []DeployRow
}

// EventRow is a Warning event: who, why, and whether it is about
// certificates. The message itself is never kept.
type EventRow struct {
	Object string
	Reason string
	Cert   bool
}

// PVCRow is a persistent volume claim and how long it has been there.
type PVCRow struct {
	Object string
	Status string
	Age    time.Duration
}

// DeployRow is a deployment: how many replicas it wants and has.
type DeployRow struct {
	Object    string
	Want      int
	Available int
	Age       time.Duration
}

// why strips "it's " from a sighting's reason.
func why(s Sighting) string {
	return strings.TrimPrefix(s.Why, "it's ")
}

// Classify picks out what is serious enough for "we need to talk".
func Classify(snap Snapshot) []TalkItem {
	var items []TalkItem
	seen := map[string]bool{}
	add := func(object, why string) {
		if object != "" && !seen[object] {
			seen[object] = true
			items = append(items, TalkItem{Object: object, Why: why})
		}
	}
	if snap.APIError != "" {
		add(apiserver, "the API server is "+snap.APIError)
		return items
	}
	for _, n := range snap.Nodes {
		if n.Trouble {
			add(n.Object, "node "+why(n))
		}
	}
	failing := map[string][]Sighting{}
	oom := 0
	for _, p := range snap.Pods {
		if !p.Trouble {
			continue
		}
		ns, _, _ := strings.Cut(p.Object, "/")
		failing[ns] = append(failing[ns], p)
		if why(p) == "OOMKilled" {
			oom++
		}
	}
	for _, ns := range sortedKeys(failing) {
		if pods := failing[ns]; len(pods) >= stormSize {
			for _, p := range pods {
				add(p.Object, fmt.Sprintf("%s (one of %d failing pods%s)", why(p), len(pods), inNamespace(ns)))
			}
		}
	}
	mounts, kills := 0, 0
	for _, e := range snap.Events {
		switch e.Reason {
		case "FailedMount", "FailedAttachVolume":
			mounts++
		case "OOMKilling":
			kills++
		}
	}
	if oom+kills >= stormSize {
		for _, p := range snap.Pods {
			if p.Trouble && why(p) == "OOMKilled" {
				add(p.Object, fmt.Sprintf("OOMKilled (out-of-memory storm: %d kills)", oom+kills))
			}
		}
		for _, e := range snap.Events {
			if e.Reason == "OOMKilling" {
				add(e.Object, fmt.Sprintf("OOMKilling (out-of-memory storm: %d kills)", oom+kills))
			}
		}
	}
	if mounts >= stormSize {
		for _, e := range snap.Events {
			if e.Reason == "FailedMount" || e.Reason == "FailedAttachVolume" {
				add(e.Object, fmt.Sprintf("%s (volume storm: %d warnings)", e.Reason, mounts))
			}
		}
	}
	for _, e := range snap.Events {
		if e.Cert {
			add(e.Object, e.Reason+" (a certificate problem)")
		}
	}
	for _, p := range snap.PVCs {
		switch {
		case p.Status == "Lost":
			add(p.Object, "volume claim Lost")
		case p.Status == "Pending" && p.Age >= pvcPendingFor:
			add(p.Object, "volume claim Pending for "+p.Age.Round(time.Minute).String())
		}
	}
	for _, d := range snap.Deploys {
		if d.Want > 0 && d.Available == 0 && d.Age >= deployGrace {
			add(d.Object, fmt.Sprintf("0/%d replicas available", d.Want))
		}
	}
	return items
}

func inNamespace(ns string) string {
	if ns == "" {
		return ""
	}
	return " in " + ns
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// updateTalk takes in what is serious now and returns what she says
// about it: "We need to talk." for something new, "Forget it." when
// everything is fine again, and nothing otherwise.
func (c *Cluster) updateTalk(context string, items []TalkItem) []string {
	t := c.State.Talk
	if len(items) == 0 {
		if t == nil || t.Fine {
			return nil
		}
		t.Fine = true
		if t.Done {
			c.State.Talk = nil
		}
		return []string{forgetIt}
	}
	if t == nil {
		for i := range items {
			items[i].Since = c.Now
		}
		c.State.Talk = &Talk{Context: context, Since: c.Now, Items: items}
		return []string{c.pick(talkLines)}
	}
	known := map[string]time.Time{}
	for _, it := range t.Items {
		known[it.Object] = it.Since
	}
	news := false
	for i := range items {
		if since, ok := known[items[i].Object]; ok {
			items[i].Since = since
		} else {
			items[i].Since = c.Now
			news = true
		}
	}
	t.Items, t.Fine, t.Context = items, false, context
	if news && t.Done {
		t.Done, t.Asked = false, 0
		return []string{c.pick(talkLines)}
	}
	return nil
}

// talkWhat answers "what's wrong?" while she needs to talk: nothing, she's
// fine, and on the third ask the whole truth.
func (c *Cluster) talkWhat() []string {
	t := c.State.Talk
	t.Asked++
	if t.Asked <= len(talkWhatAnswers) {
		return []string{talkWhatAnswers[t.Asked-1]}
	}
	lines := []string{fmt.Sprintf(talkSummaryHead, t.Context, c.when(t.Since))}
	for _, it := range t.Items {
		lines = append(lines, fmt.Sprintf("  • %s: %s, since %s → %s",
			display(it.Object), it.Why, c.when(it.Since), lookCommand(t.Context, it.Object)))
	}
	if t.Fine {
		lines = append(lines, talkAllFine)
		c.State.Talk = nil
	} else {
		t.Done = true
	}
	return lines
}

// lookCommand is the read-only command that shows more about an object.
func lookCommand(context, object string) string {
	ns, rest, _ := strings.Cut(object, "/")
	kind, name, _ := strings.Cut(rest, "/")
	cmd := "kubectl --context " + shellQuote(context)
	if ns != "" {
		cmd += " -n " + ns
	}
	switch kind {
	case "apiserver":
		return "kubectl --context " + shellQuote(context) + " cluster-info"
	case "deployment":
		return cmd + " rollout status deployment/" + name
	case "persistentvolumeclaim":
		return cmd + " describe pvc " + name
	}
	return cmd + " describe " + kind + " " + name
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9@%+=:,./_-]+$`)

func shellQuote(s string) string {
	if plainWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// --- reading what she polled ---

var certTrouble = regexp.MustCompile(`(?i)x509|certificate`)

// parseEvents reads a table of Warning events. Only the object, a plain
// reason word and whether it is about certificates are kept.
func parseEvents(out, ns string) []EventRow {
	var cols []column
	var rows []EventRow
	for _, line := range strings.Split(out, "\n") {
		if h := header(line); h != nil {
			cols = h
			continue
		}
		if cols == nil || strings.TrimSpace(line) == "" {
			continue
		}
		row := cells(cols, line)
		if row["TYPE"] != "Warning" {
			continue
		}
		rowNS := ns
		if v, ok := row["NAMESPACE"]; ok {
			rowNS = v
		}
		reason := row["REASON"]
		if !eventReason.MatchString(reason) {
			reason = "Warning"
		}
		if obj := objectOf(rowNS, "", row["OBJECT"]); obj != "" {
			rows = append(rows, EventRow{Object: obj, Reason: reason, Cert: certTrouble.MatchString(row["MESSAGE"])})
		}
	}
	return rows
}

// parsePVCs reads a table of persistent volume claims.
func parsePVCs(out, ns string) []PVCRow {
	var cols []column
	var rows []PVCRow
	for _, line := range strings.Split(out, "\n") {
		if h := header(line); h != nil {
			cols = h
			continue
		}
		if cols == nil || strings.TrimSpace(line) == "" {
			continue
		}
		row := cells(cols, line)
		rowNS := ns
		if v, ok := row["NAMESPACE"]; ok {
			rowNS = v
		}
		age, _ := parseAge(row["AGE"])
		if obj := objectOf(rowNS, "persistentvolumeclaim", row["NAME"]); obj != "" {
			rows = append(rows, PVCRow{Object: obj, Status: row["STATUS"], Age: age})
		}
	}
	return rows
}

// deployList is the part of "get deployments -o json" she reads. Field
// values, annotations' contents and the pod template are never decoded.
type deployList struct {
	Items []struct {
		Metadata struct {
			Name              string    `json:"name"`
			Namespace         string    `json:"namespace"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
		} `json:"metadata"`
		Spec struct {
			Replicas *int `json:"replicas"`
		} `json:"spec"`
		Status struct {
			AvailableReplicas int `json:"availableReplicas"`
		} `json:"status"`
	} `json:"items"`
}

// parseDeploys reads "get deployments -o json". allNS says whether the
// namespaces are to be kept (with -A) or are the current one.
func parseDeploys(out string, allNS bool, now time.Time) []DeployRow {
	var list deployList
	if json.Unmarshal([]byte(out), &list) != nil {
		return nil
	}
	var rows []DeployRow
	for _, it := range list.Items {
		ns := ""
		if allNS {
			ns = it.Metadata.Namespace
		}
		obj := objectOf(ns, "deployment", it.Metadata.Name)
		if obj == "" {
			continue
		}
		want := 1
		if it.Spec.Replicas != nil {
			want = *it.Spec.Replicas
		}
		rows = append(rows, DeployRow{Object: obj, Want: want, Available: it.Status.AvailableReplicas,
			Age: now.Sub(it.Metadata.CreationTimestamp)})
	}
	return rows
}
