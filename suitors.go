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
	"sort"
	"strings"
	"time"
)

// Other admins. Now and then she lets you know who else has been paying
// attention to her: who changed what, and when. All of it comes from the
// objects' own managedFields, read by her watcher; nothing is made up. On
// the second "misogynectl who" she tells it straight, which is also exactly
// what you need when you are looking for drift.

const (
	// suitorWindow is how recent a change must be to count.
	suitorWindow = 24 * time.Hour
	// jealousGap is the least time between two jealous remarks.
	jealousGap = 30 * time.Minute
	// mineWindow is how close to one of your own changes a change must be
	// to count as yours.
	mineWindow = 2 * time.Minute
	maxSuitors = 16
	maxMine    = 32
	maxFields  = 4
)

// managedField is one entry of metadata.managedFields. Of fieldsV1 only
// the names of the first two levels are ever looked at, never a value.
type managedField struct {
	Manager     string                     `json:"manager"`
	Operation   string                     `json:"operation"`
	Time        *time.Time                 `json:"time"`
	Subresource string                     `json:"subresource"`
	FieldsV1    map[string]json.RawMessage `json:"fieldsV1"`
}

// FieldEntry is one change someone else made: who, how, when, and roughly
// where.
type FieldEntry struct {
	Object    string    `json:"object"`
	Manager   string    `json:"manager"`
	Operation string    `json:"operation"`
	At        time.Time `json:"at"`
	Fields    []string  `json:"fields,omitempty"`
}

// Mine is a change you made yourself, through her.
type Mine struct {
	Manager string    `json:"manager"`
	At      time.Time `json:"at"`
}

// family are the cluster's own components: they live with her, they are
// not suitors.
var family = map[string]bool{
	"kube-controller-manager": true, "kubelet": true, "kube-scheduler": true,
	"kube-apiserver": true, "kube-proxy": true, "k3s": true, "before-first-apply": true,
}

var (
	managerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,63}$`)
	fieldName   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,63}$`)
)

// fieldEntries turns an object's managedFields into changes, keeping only
// names: the manager, the operation, and field paths two levels deep
// ("spec.replicas", "spec.template", "metadata.labels").
func fieldEntries(object string, mfs []managedField) []FieldEntry {
	var out []FieldEntry
	for _, mf := range mfs {
		if mf.Time == nil || !managerName.MatchString(mf.Manager) || (mf.Operation != "Apply" && mf.Operation != "Update") {
			continue
		}
		fields := coarseFields(mf.FieldsV1)
		if len(fields) == 0 {
			continue // only status: that is the family's business
		}
		out = append(out, FieldEntry{Object: object, Manager: mf.Manager, Operation: mf.Operation,
			At: *mf.Time, Fields: fields})
	}
	return out
}

func coarseFields(f map[string]json.RawMessage) []string {
	var fields []string
	for _, top := range sortedKeys(f) {
		name, ok := strings.CutPrefix(top, "f:")
		if !ok || name == "status" || !fieldName.MatchString(name) {
			continue
		}
		var sub map[string]json.RawMessage
		_ = json.Unmarshal(f[top], &sub)
		added := false
		for _, k := range sortedKeys(sub) {
			if n, ok := strings.CutPrefix(k, "f:"); ok && fieldName.MatchString(n) {
				fields = append(fields, name+"."+n)
				added = true
			}
		}
		if !added {
			fields = append(fields, name)
		}
	}
	if len(fields) > maxFields {
		fields = append(fields[:maxFields], "…")
	}
	return fields
}

// managerOf is the field manager a command of yours shows up as.
func managerOf(args []string) string {
	if m := flagValue(args, "--field-manager"); m != "" {
		return m
	}
	words := Words(args)
	if len(words) == 0 {
		return ""
	}
	if words[0] == "apply" {
		for _, a := range args {
			if a == "--server-side" || a == "--server-side=true" {
				return "kubectl"
			}
		}
		return "kubectl-client-side-apply"
	}
	return "kubectl-" + words[0]
}

// noteMine remembers that you changed something yourself, so it is never
// held up to you as someone else's attention.
func (c *Cluster) noteMine(args []string) {
	if !fixing(args) {
		return
	}
	c.State.Mine = append(c.State.Mine, Mine{Manager: managerOf(args), At: c.Now})
	if over := len(c.State.Mine) - maxMine; over > 0 {
		c.State.Mine = c.State.Mine[over:]
	}
}

// isMine reports a change made at about the time of one of yours, by your
// field manager or by kubectl in general (you cannot be told apart from
// another kubectl user any better than that).
func (c *Cluster) isMine(e FieldEntry) bool {
	for _, m := range c.State.Mine {
		d := e.At.Sub(m.At)
		if d < -mineWindow || d > mineWindow {
			continue
		}
		if e.Manager == m.Manager || (strings.HasPrefix(e.Manager, "kubectl") && strings.HasPrefix(m.Manager, "kubectl")) {
			return true
		}
	}
	return false
}

// noteSuitors takes in the changes her watcher saw: only the recent ones,
// not yours, not the family's.
func (c *Cluster) noteSuitors(entries []FieldEntry) {
	var kept []FieldEntry
	for _, e := range entries {
		if family[e.Manager] || c.isMine(e) || !c.recent(e.At, suitorWindow) {
			continue
		}
		kept = append(kept, e)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].At.After(kept[j].At) })
	if len(kept) > maxSuitors {
		kept = kept[:maxSuitors]
	}
	c.State.Suitors = kept
}

// suitors are the recent changes by others, newest first, without the
// manager of the command being typed right now.
func (c *Cluster) suitors(args []string) []FieldEntry {
	current := ""
	if fixing(args) {
		current = managerOf(args)
	}
	var out []FieldEntry
	for _, e := range c.State.Suitors {
		if e.Manager != current && !c.isMine(e) && c.recent(e.At, suitorWindow) {
			out = append(out, e)
		}
	}
	return out
}

// jealous is, now and then, a word about another admin's attention.
func (c *Cluster) jealous(args []string) string {
	if at := c.State.JealousAt; at != nil && c.recent(*at, jealousGap) {
		return ""
	}
	others := c.suitors(args)
	if len(others) == 0 || c.Rand.Intn(4) != 0 {
		return ""
	}
	e := others[c.Rand.Intn(len(others))]
	c.State.JealousAt = c.nowPtr()
	var table []string
	switch m := strings.ToLower(e.Manager); {
	case strings.Contains(m, "argocd") || strings.Contains(m, "kustomize-controller") || strings.Contains(m, "flux"):
		table = gitopsLines
	case strings.Contains(m, "helm"):
		table = helmLines
	case strings.HasPrefix(m, "kubectl"):
		table = kubectlLines
	default:
		table = suitorLines
	}
	return strings.NewReplacer("{who}", e.Manager, "{obj}", display(e.Object), "{when}", c.when(e.At),
		"{ago}", ago(c.Now.Sub(e.At)), "{fields}", strings.Join(e.Fields, ", "),
		"{how}", kubectlHow(e.Manager)).Replace(c.pick(table))
}

// kubectlHow is the command a kubectl field manager stands for.
func kubectlHow(manager string) string {
	switch manager {
	case "kubectl-client-side-apply", "kubectl":
		return "kubectl apply"
	}
	if verb, ok := strings.CutPrefix(manager, "kubectl-"); ok {
		return "kubectl " + verb
	}
	return manager
}

// ago is a rough "how long ago".
func ago(d time.Duration) string {
	switch {
	case d < 2*time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 2*time.Hour:
		return "an hour ago"
	}
	return fmt.Sprintf("%d hours ago", int(d.Hours()))
}

// Who answers "who was it?": a friend, and on the second ask the truth.
func (c *Cluster) Who() []string {
	others := c.suitors(nil)
	if len(others) == 0 {
		c.State.WhoAsked = 0
		return []string{nobody}
	}
	c.State.WhoAsked++
	if c.State.WhoAsked < 2 {
		return []string{justAFriend}
	}
	c.State.WhoAsked = 0
	lines := []string{whoTruth}
	for _, e := range others {
		lines = append(lines, fmt.Sprintf("  • %s: %s (%s) at %s, %s", display(e.Object), e.Manager, e.Operation,
			e.At.Local().Format("2006-01-02 15:04:05 MST"), strings.Join(e.Fields, ", ")))
	}
	return lines
}
