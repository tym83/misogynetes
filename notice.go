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
	"regexp"
	"strings"
	"sync"
	"time"
)

// What she notices in the output of the read commands you typed. She never
// asks kubectl anything herself: she only reads what you already asked for.

// maxSeen is how much of a command's output she reads: the first 256KB.
const maxSeen = 256 << 10

// seen keeps the head of kubectl's stdout while it goes to the terminal as
// it comes. It never fails a write, so kubectl never notices it.
type seen struct {
	mu sync.Mutex
	b  []byte
}

func (s *seen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if room := maxSeen - len(s.b); room > 0 {
		s.b = append(s.b, p[:min(len(p), room)]...)
	}
	return len(p), nil
}

func (s *seen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.b)
}

// readVerbs are the commands whose output she reads.
var readVerbs = map[string]bool{"get": true, "describe": true, "logs": true, "events": true}

// listens reports whether she reads this command's output: a read command
// printing the usual table or description, nothing interactive.
func listens(args []string) bool {
	words := Words(args)
	if len(words) == 0 || !readVerbs[words[0]] {
		return false
	}
	for i, a := range args {
		if a == "--" {
			break
		}
		name, value, hasValue := strings.Cut(a, "=")
		switch {
		case liveFlags[name] && name != "-w" && name != "--watch" && name != "--follow":
			return false
		case name == "-o" || name == "--output":
			if !hasValue {
				if i+1 >= len(args) {
					return false
				}
				value = args[i+1]
			}
			if value != "wide" {
				return false
			}
		case strings.HasPrefix(a, "-o") && !strings.HasPrefix(a, "--") && a != "-o":
			if strings.TrimPrefix(strings.TrimPrefix(a, "-o"), "=") != "wide" {
				return false
			}
		}
	}
	return true
}

// Sighting is one object as she saw it in the output.
type Sighting struct {
	Object  string // namespace/kind/name, namespace may be empty
	Trouble bool
	Healthy bool
	Why     string // what is wrong, in her words
}

// troubleStatuses are the statuses she cannot stop thinking about.
var troubleStatuses = map[string]bool{
	"CrashLoopBackOff": true, "Error": true, "OOMKilled": true, "ImagePullBackOff": true,
	"ErrImagePull": true, "CreateContainerConfigError": true, "CreateContainerError": true,
	"InvalidImageName": true, "RunContainerError": true, "ContainerStatusUnknown": true,
	"Evicted": true, "Failed": true, "BackOff": true, "NotReady": true, "DeadlineExceeded": true,
	"StartError": true, "ErrImageNeverPull": true,
}

// healthyStatuses mean it sorted itself out.
var healthyStatuses = map[string]bool{
	"Running": true, "Completed": true, "Succeeded": true, "Ready": true,
	"Active": true, "Bound": true, "Available": true,
}

// recentRestart: a restart this recent means it is still restarting.
const recentRestart = 10 * time.Minute

// kinds are the short and plural names of the kinds she cares about.
var kinds = map[string]string{
	"po": "pod", "pods": "pod", "deploy": "deployment", "deployments": "deployment",
	"rs": "replicaset", "replicasets": "replicaset", "sts": "statefulset",
	"statefulsets": "statefulset", "ds": "daemonset", "daemonsets": "daemonset",
	"no": "node", "nodes": "node", "jobs": "job", "cj": "cronjob", "cronjobs": "cronjob",
	"svc": "service", "services": "service", "pvc": "persistentvolumeclaim",
	"persistentvolumeclaims": "persistentvolumeclaim", "ns": "namespace", "namespaces": "namespace",
}

var safeName = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)

// kindOf turns "pods", "deploy", "deployment.apps" or "Pod" into one name.
func kindOf(resource string) string {
	k, _, _ := strings.Cut(strings.ToLower(resource), ".")
	if full, ok := kinds[k]; ok {
		return full
	}
	if !safeName.MatchString(k) {
		return ""
	}
	return k
}

// objectOf is what she remembers of an object: namespace, kind and name,
// all of them names Kubernetes allows, nothing that looks like a secret.
func objectOf(ns, kind, name string) string {
	if strings.Contains(name, "/") {
		k, n, _ := strings.Cut(name, "/")
		kind, name = kindOf(k), n
	}
	if kind == "" || !safeName.MatchString(name) || looksSecret(name) || (ns != "" && !safeName.MatchString(ns)) {
		return ""
	}
	return ns + "/" + kind + "/" + name
}

// Notice reads the output of a read command you typed and returns what she
// saw in it, the last sighting of each object.
func Notice(args []string, out string, now time.Time) []Sighting {
	words := Words(args)
	if len(words) == 0 {
		return nil
	}
	ns := namespaceOf(args)
	var all []Sighting
	switch words[0] {
	case "get", "events":
		kind := ""
		if len(words) > 1 && words[0] == "get" && !strings.Contains(words[1], ",") {
			kind = kindOf(words[1])
		}
		all = noticeTables(out, ns, kind, now)
	case "describe":
		kind := ""
		if len(words) > 1 {
			k, _, _ := strings.Cut(words[1], "/")
			kind = kindOf(k)
		}
		all = noticeDescribe(out, ns, kind)
	case "logs":
		all = noticeLogs(words, out, ns)
	}
	return lastOfEach(all)
}

func lastOfEach(all []Sighting) []Sighting {
	at := map[string]int{}
	var out []Sighting
	for _, s := range all {
		if s.Object == "" || (!s.Trouble && !s.Healthy) {
			continue
		}
		if i, ok := at[s.Object]; ok {
			out[i] = s
			continue
		}
		at[s.Object] = len(out)
		out = append(out, s)
	}
	return out
}

// namespaceOf is the namespace given on the command line, if any.
func namespaceOf(args []string) string {
	for i, a := range args {
		switch {
		case a == "--":
			return ""
		case (a == "-n" || a == "--namespace") && i+1 < len(args):
			return args[i+1]
		case strings.HasPrefix(a, "--namespace="):
			return strings.TrimPrefix(a, "--namespace=")
		case strings.HasPrefix(a, "-n="):
			return strings.TrimPrefix(a, "-n=")
		}
	}
	return ""
}

// headerColumn is a column name in a kubectl table header: upper-case words
// separated by single spaces ("LAST SEEN").
var headerColumn = regexp.MustCompile(`\S+( \S+)*`)
var headerLine = regexp.MustCompile(`^[A-Z0-9 ()._-]+$`)

type column struct {
	name  string
	start int
}

// header parses a kubectl table header, or returns nil for any other line.
func header(line string) []column {
	if !headerLine.MatchString(line) {
		return nil
	}
	var cols []column
	named := false
	for _, loc := range headerColumn.FindAllStringIndex(line, -1) {
		name := line[loc[0]:loc[1]]
		named = named || name == "NAME" || name == "OBJECT"
		cols = append(cols, column{name, loc[0]})
	}
	if !named {
		return nil
	}
	return cols
}

// cells cuts a table row at the header's column starts; kubectl aligns
// every cell of a column under its header.
func cells(cols []column, line string) map[string]string {
	row := map[string]string{}
	for i, c := range cols {
		if c.start >= len(line) {
			break
		}
		end := len(line)
		if i+1 < len(cols) && cols[i+1].start < end {
			end = cols[i+1].start
		}
		row[c.name] = strings.TrimSpace(line[c.start:end])
	}
	return row
}

func noticeTables(out, ns, kind string, now time.Time) []Sighting {
	var cols []column
	var all []Sighting
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			cols = nil
			continue
		}
		if h := header(line); h != nil {
			cols = h
			continue
		}
		if cols == nil {
			continue
		}
		row := cells(cols, line)
		rowNS := ns
		if v, ok := row["NAMESPACE"]; ok {
			rowNS = v
		}
		if obj, ok := row["OBJECT"]; ok {
			if row["TYPE"] == "Warning" {
				all = append(all, Sighting{Object: objectOf(rowNS, "", obj), Trouble: true, Why: keepsSaying(row["REASON"])})
			}
			continue
		}
		all = append(all, rowSighting(objectOf(rowNS, kind, row["NAME"]), row, now))
	}
	return all
}

// rowSighting judges one row of "kubectl get".
func rowSighting(obj string, row map[string]string, now time.Time) Sighting {
	s := Sighting{Object: obj}
	status, hasStatus := row["STATUS"]
	ready, hasReady := row["READY"]
	for _, part := range strings.Split(status, ",") {
		part = strings.TrimPrefix(part, "Init:")
		if troubleStatuses[part] {
			s.Trouble, s.Why = true, "it's "+part
			return s
		}
	}
	if restartedRecently(row["RESTARTS"]) {
		s.Trouble, s.Why = true, "it keeps restarting"
		return s
	}
	first, _, _ := strings.Cut(status, ",")
	if first == "Completed" || first == "Succeeded" {
		s.Healthy = true // done, so nothing is ready any more
		return s
	}
	s.Healthy = (hasStatus || hasReady) &&
		(!hasStatus || healthyStatuses[first]) &&
		(!hasReady || balanced(ready))
	return s
}

// balanced reports a READY column like "3/3".
func balanced(ready string) bool {
	a, b, ok := strings.Cut(ready, "/")
	return ok && a == b
}

var restarts = regexp.MustCompile(`^(\d+) \((\S+) ago\)$`)

// restartedRecently reads a RESTARTS cell like "5 (2m30s ago)".
func restartedRecently(cell string) bool {
	m := restarts.FindStringSubmatch(cell)
	if m == nil || m[1] == "0" {
		return false
	}
	ago, ok := parseAge(m[2])
	return ok && ago < recentRestart
}

var ageUnits = map[byte]time.Duration{'s': time.Second, 'm': time.Minute, 'h': time.Hour,
	'd': 24 * time.Hour, 'y': 365 * 24 * time.Hour}

// parseAge reads kubectl's short durations: 45s, 2m30s, 3h5m, 4d, 2y30d.
func parseAge(s string) (time.Duration, bool) {
	var total time.Duration
	n := -1
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			if n < 0 {
				n = 0
			}
			n = n*10 + int(ch-'0')
			continue
		}
		unit, ok := ageUnits[ch]
		if !ok || n < 0 {
			return 0, false
		}
		total += time.Duration(n) * unit
		n = -1
	}
	return total, n < 0 && s != ""
}

var eventReason = regexp.MustCompile(`^[A-Za-z]{1,40}$`)

// keepsSaying is a Warning event, in her words. Only a plain reason word is
// repeated, never an event's message.
func keepsSaying(reason string) string {
	if !eventReason.MatchString(reason) {
		reason = "Warning"
	}
	return "it keeps saying " + reason
}

// noticeDescribe reads "kubectl describe": every object starts with a
// "Name:" line of its own.
func noticeDescribe(out, ns, kind string) []Sighting {
	var all []Sighting
	var b *describeBlock
	flush := func() {
		if b != nil {
			all = append(all, b.sighting())
		}
	}
	for _, line := range strings.Split(out, "\n") {
		key, value, _ := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		if key == "Name" {
			flush()
			b = &describeBlock{object: value, ns: ns, kind: kind}
			continue
		}
		if b != nil {
			b.read(line)
		}
	}
	flush()
	return all
}

type describeBlock struct {
	object, ns, kind string
	status           string
	trouble          string
	warning          string
	notReady         bool
	replicas         bool
	unavailable      bool
	inLastState      bool
	inEvents         bool
}

func (b *describeBlock) read(line string) {
	trimmed := strings.TrimSpace(line)
	key, value, _ := strings.Cut(trimmed, ":")
	value = strings.TrimSpace(value)
	top := line == trimmed
	if top {
		b.inEvents = key == "Events"
		switch key {
		case "Namespace":
			b.ns = value
		case "Status":
			b.status = value
			if value == "Failed" && b.trouble == "" {
				b.trouble = "it's Failed"
			}
		case "Replicas":
			b.replicas = true
			b.unavailable = !strings.Contains(value, " 0 unavailable") && strings.Contains(value, "unavailable")
		}
	}
	if b.inEvents {
		if f := strings.Fields(trimmed); len(f) > 1 && f[0] == "Warning" && b.warning == "" {
			b.warning = keepsSaying(f[1])
		}
		return
	}
	switch key {
	case "Last State":
		b.inLastState = true
	case "State":
		b.inLastState = false
	case "Reason":
		if !b.inLastState && troubleStatuses[value] && b.trouble == "" {
			b.trouble = "it's " + value
		}
	case "Ready":
		b.notReady = b.notReady || value == "False"
	}
}

func (b *describeBlock) sighting() Sighting {
	s := Sighting{Object: objectOf(b.ns, b.kind, b.object)}
	s.Healthy = b.trouble == "" && !b.notReady &&
		(healthyStatuses[b.status] || (b.replicas && !b.unavailable))
	switch {
	case b.trouble != "":
		s.Trouble, s.Why = true, b.trouble
	case b.warning != "" && !s.Healthy:
		s.Trouble, s.Why = true, b.warning
	}
	return s
}

var logTrouble = regexp.MustCompile(`(?i)\b(error|fatal|panic|exception|oomkilled|out of memory|segmentation fault)\b`)

func noticeLogs(words []string, out, ns string) []Sighting {
	if len(words) < 2 || !logTrouble.MatchString(out) {
		return nil
	}
	kind, name := "pod", words[1]
	if k, n, ok := strings.Cut(words[1], "/"); ok {
		kind, name = kindOf(k), n
	}
	return []Sighting{{Object: objectOf(ns, kind, name), Trouble: true, Why: "its logs are full of errors"}}
}

// display is an object the way she says it: kind/name, and the namespace.
func display(object string) string {
	ns, rest, _ := strings.Cut(object, "/")
	if ns == "" {
		return rest
	}
	return rest + " in " + ns
}
