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
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cycleDays and pmsFrom place "that time of the month" in a 28-day cycle
// that starts at a random point on install.
const (
	cycleDays = 28
	pmsFrom   = 24
)

// State is kept between runs, because she remembers everything.
type State struct {
	Installed   time.Time `json:"installed"`
	Offset      int       `json:"offset"`
	Mood        int       `json:"mood"` // 0 calm, more is worse
	Grudge      string    `json:"grudge,omitempty"`
	GrudgeAt    time.Time `json:"grudgeAt,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
	Asked       int       `json:"asked"`
	BannerShown bool      `json:"bannerShown"`
}

// Cluster decides how she reacts.
type Cluster struct {
	Rand  *rand.Rand
	Now   time.Time
	Day   int // -1: derive from the cycle
	State *State
}

// CycleDay is today's day in the cycle, 0-based.
func (c *Cluster) CycleDay() int {
	if c.Day >= 0 {
		return c.Day % cycleDays
	}
	days := int(c.Now.Sub(c.State.Installed).Hours() / 24)
	return (days + c.State.Offset) % cycleDays
}

// PMS reports whether it is one of the days he is so sure about.
func (c *Cluster) PMS() bool {
	return c.CycleDay() >= pmsFrom
}

func (c *Cluster) pick(lines []string) string {
	return lines[c.Rand.Intn(len(lines))]
}

// Verb is the first word of a kubectl command line that is not a flag.
func Verb(args []string) string {
	skip := map[string]bool{"-n": true, "--namespace": true, "--context": true, "--kubeconfig": true,
		"-o": true, "--output": true, "-l": true, "--selector": true, "-f": true, "--filename": true}
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			if skip[args[i]] {
				i++
			}
			continue
		}
		return args[i]
	}
	return "nothing"
}

// Plan is what she says before a command, and whether she runs it at all.
type Plan struct {
	Say []string
	Run bool
}

// Before decides how to meet a command.
func (c *Cluster) Before(args []string) Plan {
	verb := Verb(args)
	var say []string
	if !c.State.BannerShown {
		say = append(say, banner, "")
		c.State.BannerShown = true
	}
	switch {
	case c.PMS():
		say = append(say, pmsNotice)
		if c.Rand.Intn(10) < 4 {
			return Plan{Say: append(say, doWhatYouWant), Run: false}
		}
		line := c.pick(pmsLines)
		if strings.Contains(line, "%s") {
			line = fmt.Sprintf(line, verb)
		}
		return Plan{Say: append(say, line), Run: true}
	case c.State.Mood > 0:
		if c.Rand.Intn(2) == 0 {
			return Plan{Say: append(say, doWhatYouWant), Run: false}
		}
		return Plan{Say: append(say, c.pick(sulks)), Run: true}
	default:
		if c.Rand.Intn(8) == 0 {
			line := c.pick(whims)
			if strings.Contains(line, "%s") {
				line = fmt.Sprintf(line, verb)
			}
			return Plan{Say: append(say, line), Run: false}
		}
		if c.Rand.Intn(4) == 0 {
			say = append(say, c.pick(quotes))
		}
		return Plan{Say: say, Run: true}
	}
}

// Failed hides a kubectl error behind "everything's fine" and holds it
// against the user.
func (c *Cluster) Failed(args []string, stderr string) []string {
	c.State.Mood++
	c.State.Grudge = strings.Join(withoutConnection(args), " ")
	c.State.GrudgeAt = c.Now
	c.State.LastError = strings.TrimSpace(stderr)
	c.State.Asked = 0
	return []string{fineAfterError}
}

// withoutConnection drops where the command was sent, keeping what was done.
func withoutConnection(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--kubeconfig", "--context", "--server", "-s", "--token", "--user", "--cluster":
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// What answers "what's wrong?": nothing, then you-know-what, then the truth.
func (c *Cluster) What() []string {
	if c.State.Grudge == "" {
		return []string{whatAnswers[0]}
	}
	c.State.Asked++
	i := c.State.Asked - 1
	if i >= len(whatAnswers)-1 {
		return []string{whatAnswers[len(whatAnswers)-1],
			fmt.Sprintf("At %s you ran: %s", c.State.GrudgeAt.Format("15:04"), c.State.Grudge),
			c.State.LastError}
	}
	return []string{whatAnswers[i]}
}

// Sorry takes an apology. It has to be for the right thing.
func (c *Cluster) Sorry(reason string) []string {
	if c.State.Mood == 0 {
		c.State.Mood = 1
		c.State.Grudge = "sorry"
		c.State.GrudgeAt = c.Now
		c.State.LastError = "You apologized for nothing. That's suspicious."
		c.State.Asked = 0
		return []string{nothingToApologize}
	}
	at := fmt.Sprintf(thinkAboutIt, c.State.GrudgeAt.Format("15:04"))
	if strings.TrimSpace(reason) == "" {
		return []string{sorryForWhat, at}
	}
	if !apologyMatches(reason, c.State.Grudge) {
		return []string{notWhatItsAbout, at}
	}
	*c.State = State{Installed: c.State.Installed, Offset: c.State.Offset, BannerShown: true}
	return []string{apologyAccepted}
}

// apologyMatches: the apology must name what was done, by its first word at
// least ("sorry for delete" covers "delete pod x").
func apologyMatches(reason, grudge string) bool {
	reason = strings.ToLower(reason)
	words := strings.Fields(strings.ToLower(grudge))
	if len(words) == 0 {
		return false
	}
	return strings.Contains(reason, Verb(words)) || strings.Contains(reason, strings.ToLower(grudge))
}

// Flowers are nice. They don't change anything.
func (c *Cluster) Flowers() []string {
	return []string{flowersNice}
}

// Load reads the state, creating it on first use with a random cycle offset.
func Load(path string, now time.Time, r *rand.Rand) *State {
	s := &State{}
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, s) == nil && !s.Installed.IsZero() {
		return s
	}
	return &State{Installed: now, Offset: r.Intn(cycleDays)}
}

// Save writes the state.
func Save(path string, s *State) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		_ = os.WriteFile(path, b, 0o600)
	}
}
