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
		lines = append(lines, c.pick(talkLines))
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
