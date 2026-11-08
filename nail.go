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
	"regexp"
	"strings"
	"time"
)

// "It's Not About the Nail" (Jason Headley, 2013): she has a nail in her
// forehead and wants to be heard; he keeps trying to fix it. Here she tells
// you about the trouble she sees in the output of your own read commands,
// and you are supposed to listen, not fix.

// nailExit is the exit code when she refuses a fix because she only wanted
// to share: EX_TEMPFAIL, "try again later". Nothing ran.
const nailExit = 75

const (
	// shareWindow is how soon after you heard it the same trouble is not
	// worth announcing again. Unheard trouble never goes quiet.
	shareWindow = 15 * time.Minute
	// insistWindow is how soon the same fix must be typed again to insist.
	insistWindow = 2 * time.Minute
	// forgetAfter is when she lets go of a problem nobody ever saw fixed.
	forgetAfter = 24 * time.Hour
	// maxShared is how many problems she keeps in mind at once.
	maxShared = 16
)

// Share is a problem she told you about.
type Share struct {
	Object string    `json:"object"` // namespace/kind/name
	Why    string    `json:"why"`
	At     time.Time `json:"at"`
	Heard  bool      `json:"heard,omitempty"`
	Healed bool      `json:"healed,omitempty"` // sorted itself out, still unheard
}

// Hurt is the fix she refused, waiting to see if you insist.
type Hurt struct {
	Command string    `json:"command"` // verb, resource, name: as in a grudge
	At      time.Time `json:"at"`
}

// fixVerbs change something, which is not what she asked for.
var fixVerbs = map[string]bool{
	"delete": true, "edit": true, "patch": true, "scale": true, "autoscale": true,
	"apply": true, "replace": true, "create": true, "set": true, "drain": true,
	"cordon": true, "uncordon": true, "taint": true, "label": true, "annotate": true,
	"exec": true, "debug": true, "cp": true, "run": true, "expose": true,
}

// fixRollouts are the rollout subcommands that change something.
var fixRollouts = map[string]bool{"restart": true, "undo": true, "pause": true, "resume": true}

// fixing reports a command that tries to fix things.
func fixing(args []string) bool {
	words := Words(args)
	if len(words) == 0 {
		return false
	}
	if words[0] == "rollout" {
		return len(words) > 1 && fixRollouts[words[1]]
	}
	return fixVerbs[words[0]]
}

// unheard reports a problem she shared that you have not listened to. It
// does not go away with time, only with "aga" (or after forgetAfter).
func (c *Cluster) unheard() bool {
	if t := c.State.Talk; t != nil && !t.Done {
		return true
	}
	for _, s := range c.State.Shared {
		if !s.Heard {
			return true
		}
	}
	return false
}

func (c *Cluster) recent(at time.Time, window time.Duration) bool {
	d := c.Now.Sub(at)
	return d >= 0 && d < window
}

// beforeNail meets a command while she has something on her mind. It
// returns false when the nail has nothing to do with this command.
func (c *Cluster) beforeNail(args []string, say []string) (Plan, bool) {
	c.forget()
	if fixing(args) && c.unheard() {
		cmd := grudgeOf(args)
		if h := c.State.Hurt; h != nil && h.Command == cmd && c.recent(h.At, insistWindow) {
			c.State.Hurt, c.State.Sulking = nil, true
			return Plan{Say: append(say, insisted), Run: true}, true
		}
		c.State.Hurt = &Hurt{Command: cmd, At: c.Now}
		return Plan{Say: append(say, c.pick(hurtLines), fmt.Sprintf(hurtHint, nailExit)), Code: nailExit}, true
	}
	if lines := c.Remind(); len(lines) > 0 {
		return Plan{Say: append(say, lines...), Run: true}, true
	}
	return Plan{}, false
}

// Remind is what she says on any command while she is sulking or still
// waiting to be heard. Every command that ignores her takes her one level
// further; the top level repeats.
func (c *Cluster) Remind() []string {
	var lines []string
	if c.State.Sulking {
		lines = append(lines, c.pick(curtLines))
	}
	return append(lines, c.nag()...)
}

// nag is the reminder itself, on a command or when she comes running.
func (c *Cluster) nag() []string {
	waiting := c.waiting()
	talk := c.State.Talk != nil && !c.State.Talk.Done
	if len(waiting) == 0 && !talk {
		return nil
	}
	c.State.Ignored++
	c.State.NaggedAt = c.nowPtr()
	var lines []string
	if talk {
		// She does not say what it is about.
		lines = append(lines, c.talkLine())
	}
	if len(waiting) == 0 {
		return lines
	}
	var line string
	switch n := c.State.Ignored; {
	case n <= len(reminders):
		line = c.pick(reminders[n-1])
	case n%2 == 0:
		line = c.pick(neverMind)
	default:
		line = c.pick(pointedReminders)
	}
	return append(lines, strings.ReplaceAll(line, "{obj}", waitingFor(waiting)))
}

// waiting is every object she is still waiting to be heard about.
func (c *Cluster) waiting() []string {
	var objs []string
	for _, s := range c.State.Shared {
		if !s.Heard {
			objs = append(objs, display(s.Object))
		}
	}
	return objs
}

// waitingFor names the first two of them, and how many more.
func waitingFor(objs []string) string {
	if len(objs) <= 2 {
		return strings.Join(objs, " and ")
	}
	return strings.Join(objs[:2], ", ") + fmt.Sprintf(andOthers, len(objs)-2)
}

// Aga is you, listening: "uh-huh". Nothing runs.
func (c *Cluster) Aga() []string {
	c.forget()
	sulking := c.State.Sulking || c.State.Hurt != nil
	unheard := c.unheard()
	c.State.Sulking, c.State.Hurt, c.State.Ignored = false, nil, 0
	if t := c.State.Talk; t != nil {
		t.Done = true
		if t.Fine {
			c.State.Talk = nil
		}
	}
	var healed []string
	kept := c.State.Shared[:0]
	for _, s := range c.State.Shared {
		if s.Healed {
			healed = append(healed, display(s.Object))
			continue
		}
		s.Heard = true
		kept = append(kept, s)
	}
	c.State.Shared = kept
	if len(kept) == 0 {
		c.State.Shared = nil
	}
	var lines []string
	switch {
	case sulking:
		lines = append(lines, sulkOver)
	case unheard:
		lines = append(lines, c.pick(heardLines))
	case len(c.State.Shared) > 0:
		lines = append(lines, stillHeard)
	default:
		return []string{agaNothing}
	}
	if len(healed) > 0 {
		lines = append(lines, fmt.Sprintf(sortedItself, waitingFor(healed)))
	}
	return lines
}

// Observe takes in what she saw in the output of your read command: new
// trouble she shares, remembered trouble that sorted itself out.
func (c *Cluster) Observe(args []string, out string) []string {
	return c.observe(Notice(args, out, c.Now))
}

// observe takes in sightings, from your commands or from her own looking.
func (c *Cluster) observe(sightings []Sighting) []string {
	c.forget()
	var shared []Share
	var lines []string
	for _, s := range sightings {
		i := c.find(s.Object)
		var known *Share
		if i >= 0 {
			known = &c.State.Shared[i]
		}
		switch {
		case s.Trouble && known != nil && !known.Heard:
			// Still the same nail, and she is still telling you about it.
			known.Healed = false
		case s.Trouble && known != nil && c.recent(known.At, shareWindow):
			// You heard it a moment ago.
		case s.Trouble:
			sh := Share{Object: s.Object, Why: s.Why, At: c.Now}
			if known != nil {
				*known = sh
			} else {
				c.State.Shared = append(c.State.Shared, sh)
			}
			shared = append(shared, sh)
		case s.Healthy && known != nil && !known.Heard:
			// It sorted itself out, but she still wants to be heard.
			if !known.Healed {
				known.Healed = true
				lines = append(lines, fmt.Sprintf(sortedUnheard, display(known.Object)))
			}
		case s.Healthy && known != nil:
			obj := known.Object
			c.State.Shared = append(c.State.Shared[:i], c.State.Shared[i+1:]...)
			lines = append(lines, fmt.Sprintf(sortedItself, display(obj)), c.pick(onlyListened))
		}
	}
	if over := len(c.State.Shared) - maxShared; over > 0 {
		c.State.Shared = c.State.Shared[over:]
	}
	if len(shared) == 0 {
		return lines
	}
	line := strings.NewReplacer("{obj}", display(shared[0].Object), "{why}", shared[0].Why).Replace(c.pick(shareLines))
	if more := len(shared) - 1; more > 0 {
		line += fmt.Sprintf(andMore, more)
	}
	return append(lines, line, fmt.Sprintf(shareHint, nailExit))
}

// find is the remembered problem for an object. A namespace left out on
// either side matches any: "get pods" means the current namespace.
func (c *Cluster) find(object string) int {
	ns, rest, _ := strings.Cut(object, "/")
	for i, s := range c.State.Shared {
		sns, srest, _ := strings.Cut(s.Object, "/")
		if srest == rest && (sns == ns || sns == "" || ns == "") {
			return i
		}
	}
	return -1
}

// forget lets go of problems nobody saw sorted out for a day, and of a
// refused fix too old to insist on.
func (c *Cluster) forget() {
	kept := c.State.Shared[:0]
	for _, s := range c.State.Shared {
		if c.recent(s.At, forgetAfter) {
			kept = append(kept, s)
		}
	}
	c.State.Shared = kept
	if len(kept) == 0 {
		c.State.Shared = nil
	}
	if h := c.State.Hurt; h != nil && !c.recent(h.At, insistWindow) {
		c.State.Hurt = nil
	}
	if !c.unheard() {
		c.State.Ignored = 0
	}
}

// Gone lets go of what she was waiting to be heard about when the object
// itself is gone: she does not keep bringing up a pod that no longer
// exists. Being hurt, or sulking, is another matter and stays.
func (c *Cluster) Gone(objects []string) {
	for _, o := range objects {
		for i := c.find(o); i >= 0; i = c.find(o) {
			c.State.Shared = append(c.State.Shared[:i], c.State.Shared[i+1:]...)
		}
	}
	if len(c.State.Shared) == 0 {
		c.State.Shared = nil
	}
	c.forget()
}

// deletedObjects are the objects a delete command names: "delete pod a b",
// "delete pod/a deploy/b". Selectors and files name nothing she can know.
func deletedObjects(args []string) []string {
	words := Words(args)
	if len(words) < 2 || words[0] != "delete" {
		return nil
	}
	ns := namespaceOf(args)
	var objs []string
	add := func(o string) {
		if o != "" {
			objs = append(objs, o)
		}
	}
	if strings.Contains(words[1], "/") {
		for _, w := range words[1:] {
			if strings.Contains(w, "/") {
				add(objectOf(ns, "", w))
			}
		}
		return objs
	}
	if strings.Contains(words[1], ",") {
		return nil
	}
	kind := kindOf(words[1])
	for _, name := range words[2:] {
		add(objectOf(ns, kind, name))
	}
	return objs
}

// deletedLine is how kubectl reports an object it deleted: `pod "web-1"
// deleted`, newer versions add `from shop namespace`. Dry runs end in
// "(dry run)" and do not match.
var deletedLine = regexp.MustCompile(`^([A-Za-z0-9.-]+) "([^"]+)" deleted(?: from ([a-z0-9-]+) namespace)?$`)

// deletedNameLine is the same with "-o name": `pod/web-1`.
var deletedNameLine = regexp.MustCompile(`^([A-Za-z0-9.-]+)/([a-z0-9.-]+)$`)

// deletedInOutput are the objects kubectl says it deleted, whatever picked
// them: names, "-l" selectors, "-f" files, "--all". She takes kubectl's
// word for it rather than guessing from the command line.
func deletedInOutput(args []string, stdout string) []string {
	if Verb(args) != "delete" {
		return nil
	}
	ns := namespaceOf(args)
	nameOutput := outputFormat(args) == "name"
	var objs []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		var o string
		if m := deletedLine.FindStringSubmatch(line); m != nil {
			objNS := ns
			if m[3] != "" {
				objNS = m[3]
			}
			o = objectOf(objNS, kindOf(m[1]), m[2])
		} else if m := deletedNameLine.FindStringSubmatch(line); m != nil && nameOutput {
			o = objectOf(ns, kindOf(m[1]), m[2])
		}
		if o != "" {
			objs = append(objs, o)
		}
	}
	return objs
}

// outputFormat is the value of -o/--output, or "".
func outputFormat(args []string) string {
	for i, a := range args {
		if a == "--" {
			break
		}
		name, value, hasValue := strings.Cut(a, "=")
		switch {
		case name == "-o" || name == "--output":
			if !hasValue && i+1 < len(args) {
				value = args[i+1]
			}
			return value
		case strings.HasPrefix(a, "-o") && !strings.HasPrefix(a, "--"):
			return strings.TrimPrefix(strings.TrimPrefix(a, "-o"), "=")
		}
	}
	return ""
}

// orphans reports "--cascade=orphan": the dependents stay behind.
func orphans(args []string) bool {
	for i, a := range args {
		if a == "--" {
			break
		}
		if a == "--cascade=orphan" || (a == "--cascade" && i+1 < len(args) && args[i+1] == "orphan") {
			return true
		}
	}
	return false
}

// hashChars are the characters Kubernetes uses for generated name suffixes
// and pod-template hashes.
const hashChars = `[bcdfghjklmnpqrstvwxz2456789]`

type dependent struct {
	kind   string
	suffix *regexp.Regexp
}

func suffix(re string) *regexp.Regexp { return regexp.MustCompile(`^` + re + `$`) }

// dependents are the names a controller gives what it creates, by the kind
// of the controller: the kind of the dependent and its name after
// "<controller name>-".
var dependents = map[string][]dependent{
	"deployment": {
		{"replicaset", suffix(hashChars + `{1,10}`)},
		{"pod", suffix(hashChars + `{1,10}-` + hashChars + `{5}`)},
	},
	"replicaset":  {{"pod", suffix(hashChars + `{5}`)}},
	"daemonset":   {{"pod", suffix(hashChars + `{5}`)}},
	"job":         {{"pod", suffix(hashChars + `{5}`)}},
	"statefulset": {{"pod", suffix(`[0-9]+`)}},
	"cronjob": {
		{"job", suffix(`[0-9]+`)},
		{"pod", suffix(`[0-9]+-` + hashChars + `{5}`)},
	},
}

// ownedBy reports whether object (namespace/kind/name) is named the way the
// controller (namespace/kind/name) names what it creates, in its namespace.
func ownedBy(object, controller string) bool {
	cns, ckind, cname := splitObject(controller)
	ons, okind, oname := splitObject(object)
	if cns != ons && cns != "" && ons != "" {
		return false
	}
	rest, ok := strings.CutPrefix(oname, cname+"-")
	if !ok {
		return false
	}
	for _, d := range dependents[ckind] {
		if d.kind == okind && d.suffix.MatchString(rest) {
			return true
		}
	}
	return false
}

func splitObject(o string) (ns, kind, name string) {
	ns, rest, _ := strings.Cut(o, "/")
	kind, name, _ = strings.Cut(rest, "/")
	return ns, kind, name
}

// GoneWithDependents is Gone for objects deleted with the default cascade:
// a deleted Deployment takes its ReplicaSets and pods with it, so she
// stops bringing those up too.
func (c *Cluster) GoneWithDependents(objects []string) {
	var all []string
	for _, s := range c.State.Shared {
		for _, o := range objects {
			if ownedBy(s.Object, o) {
				all = append(all, s.Object)
				break
			}
		}
	}
	c.Gone(append(all, objects...))
}

var notFoundLine = regexp.MustCompile(`\(NotFound\): ([A-Za-z0-9.-]+) "([^"]+)" not found`)

// notFound are the objects kubectl said do not exist.
func notFound(args []string, stderr string) []string {
	ns := namespaceOf(args)
	var objs []string
	for _, m := range notFoundLine.FindAllStringSubmatch(stderr, -1) {
		if o := objectOf(ns, kindOf(m[1]), m[2]); o != "" {
			objs = append(objs, o)
		}
	}
	return objs
}
