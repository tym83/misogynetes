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
	// shareWindow is how long a problem she shared waits to be heard.
	shareWindow = 15 * time.Minute
	// insistWindow is how soon the same fix must be typed again to insist.
	insistWindow = 2 * time.Minute
	// curtRuns is how many commands get curt replies after you insisted.
	curtRuns = 2
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

// unheard reports a problem she shared recently that you have not listened to.
func (c *Cluster) unheard() bool {
	for _, s := range c.State.Shared {
		if !s.Heard && c.recent(s.At, shareWindow) {
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
	if fixing(args) && c.unheard() {
		cmd := grudgeOf(args)
		if h := c.State.Hurt; h != nil && h.Command == cmd && c.recent(h.At, insistWindow) {
			c.State.Hurt, c.State.Shared, c.State.Curt = nil, nil, curtRuns
			return Plan{Say: append(say, insisted), Run: true}, true
		}
		c.State.Hurt = &Hurt{Command: cmd, At: c.Now}
		return Plan{Say: append(say, c.pick(hurtLines), fmt.Sprintf(hurtHint, nailExit)), Code: nailExit}, true
	}
	if c.State.Curt > 0 {
		c.State.Curt--
		return Plan{Say: append(say, c.pick(curtLines)), Run: true}, true
	}
	return Plan{}, false
}

// Aga is you, listening: "uh-huh". Nothing runs.
func (c *Cluster) Aga() []string {
	c.forget()
	sulking := c.State.Curt > 0 || c.State.Hurt != nil
	c.State.Curt, c.State.Hurt = 0, nil
	unheard := c.unheard()
	for i := range c.State.Shared {
		c.State.Shared[i].Heard = true
	}
	switch {
	case sulking:
		return []string{sulkOver}
	case unheard:
		return []string{c.pick(heardLines)}
	case len(c.State.Shared) > 0:
		return []string{stillHeard}
	}
	return []string{agaNothing}
}

// Observe takes in what she saw in the output of your read command: new
// trouble she shares, remembered trouble that sorted itself out.
func (c *Cluster) Observe(args []string, out string) []string {
	c.forget()
	var shared []Share
	var lines []string
	for _, s := range Notice(args, out, c.Now) {
		i := c.find(s.Object)
		switch {
		case s.Trouble && i >= 0 && c.recent(c.State.Shared[i].At, shareWindow):
			// Still the same nail. She already told you.
		case s.Trouble:
			sh := Share{Object: s.Object, Why: s.Why, At: c.Now}
			if i >= 0 {
				c.State.Shared[i] = sh
			} else {
				c.State.Shared = append(c.State.Shared, sh)
			}
			shared = append(shared, sh)
		case s.Healthy && i >= 0:
			sh := c.State.Shared[i]
			c.State.Shared = append(c.State.Shared[:i], c.State.Shared[i+1:]...)
			lines = append(lines, fmt.Sprintf(sortedItself, display(sh.Object)))
			if sh.Heard {
				lines = append(lines, c.pick(onlyListened))
			}
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
}
