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
	"math/rand"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// cycleDays and pmsFrom place "that time of the month" in a 28-day cycle
// that starts at a random point on install.
const (
	cycleDays = 28
	pmsFrom   = 24
)

// State is kept between runs, because she remembers everything.
type State struct {
	Installed   time.Time  `json:"installed"`
	Offset      int        `json:"offset"`
	Mood        int        `json:"mood"` // 0 calm, more is worse
	Grudge      string     `json:"grudge,omitempty"`
	GrudgeAt    *time.Time `json:"grudgeAt,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
	Asked       int        `json:"asked"`
	BannerShown bool       `json:"bannerShown"`
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
	if days < 0 { // the clock went back past the install
		days = 0
	}
	return ((days+c.State.Offset)%cycleDays + cycleDays) % cycleDays
}

// PMS reports whether it is one of the days he is so sure about.
func (c *Cluster) PMS() bool {
	return c.CycleDay() >= pmsFrom
}

func (c *Cluster) pick(lines []string) string {
	return lines[c.Rand.Intn(len(lines))]
}

// valueFlags are the kubectl flags that take a value as the next word when
// written without "=". The global ones come first; a few common per-command
// ones follow, so that their values are not mistaken for words either.
var valueFlags = map[string]bool{
	"--as": true, "--as-group": true, "--as-uid": true, "--cache-dir": true,
	"--certificate-authority": true, "--client-certificate": true, "--client-key": true,
	"--cluster": true, "--context": true, "--kubeconfig": true, "--kuberc": true,
	"-n": true, "--namespace": true, "--password": true, "--profile": true,
	"--profile-output": true, "--request-timeout": true, "-s": true, "--server": true,
	"--tls-server-name": true, "--token": true, "--user": true, "--username": true,
	"-v": true, "--v": true, "--vmodule": true, "--log-dir": true, "--log-file": true,
	"--log-file-max-size": true, "--log-flush-frequency": true, "--log-backtrace-at": true,
	"--stderrthreshold": true,

	"-o": true, "--output": true, "-l": true, "--selector": true, "-f": true, "--filename": true,
	"-c": true, "--container": true, "--field-selector": true, "--image": true,
	"--from-literal": true, "--from-file": true, "--from-env-file": true, "--type": true,
}

// Words are the words of a kubectl command line that are not flags or flag
// values: the verb, the resource, the name and so on.
func Words(args []string) []string {
	var words []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return append(words, args[i+1:]...)
		case strings.HasPrefix(a, "-") && len(a) > 1:
			if !strings.Contains(a, "=") && valueFlags[a] {
				i++
			}
		default:
			words = append(words, a)
		}
	}
	return words
}

// Verb is the first word of a kubectl command line that is not a flag or a
// flag's value.
func Verb(args []string) string {
	if w := Words(args); len(w) > 0 {
		return w[0]
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
// against the user. Only a short, redacted form of both is kept.
func (c *Cluster) Failed(args []string, stderr string) []string {
	c.State.Mood++
	c.State.Grudge = grudgeOf(args)
	c.State.GrudgeAt = c.nowPtr()
	c.State.LastError = redactError(stderr)
	c.State.Asked = 0
	return []string{fineAfterError}
}

// grudgeWords is how much of a command she remembers: verb, resource, name.
const grudgeWords = 3

// grudgeOf is what she remembers of a command: the verb, the resource and
// the name, never a flag or a flag's value, so no token, password, server
// or kubeconfig ends up in the state file.
func grudgeOf(args []string) string {
	var kept []string
	for _, w := range Words(args) {
		if len(kept) == grudgeWords {
			break
		}
		if strings.Contains(w, "=") || looksSecret(w) {
			continue
		}
		kept = append(kept, w)
	}
	return strings.Join(kept, " ")
}

var (
	jwtLike       = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{4,}(\.[A-Za-z0-9_-]*){0,2}`)
	bootstrapLike = regexp.MustCompile(`\b[a-z0-9]{6}\.[a-z0-9]{16}\b`)
	longOpaque    = regexp.MustCompile(`^[A-Za-z0-9+/_]{32,}={0,2}$`)
	bearerValue   = regexp.MustCompile(`(?i)(bearer|basic)\s+[A-Za-z0-9._~+/=-]+`)
	secretFlag    = regexp.MustCompile(`(?i)(--?(token|password|username|client-key|client-certificate|kubeconfig|server|as|as-group|as-uid|from-literal|from-file|from-env-file)[= ])\S+`)
	secretField   = regexp.MustCompile(`(?i)\b(token|password|secret)(["']?\s*[:=]\s*["']?)[^\s"',}]+`)
)

// looksSecret reports a word that looks like a credential rather than a name.
func looksSecret(w string) bool {
	return jwtLike.MatchString(w) || bootstrapLike.MatchString(w) || longOpaque.MatchString(w)
}

// maxLastError is how much of a kubectl error she keeps.
const maxLastError = 2048

// redactError trims a kubectl error and blanks out anything that looks like
// a credential before it is written down.
func redactError(stderr string) string {
	s := strings.TrimSpace(truncate(stderr, 4*maxLastError))
	s = bearerValue.ReplaceAllString(s, "$1 <redacted>")
	s = secretFlag.ReplaceAllString(s, "$1<redacted>")
	s = secretField.ReplaceAllString(s, "$1$2<redacted>")
	s = jwtLike.ReplaceAllString(s, "<redacted>")
	s = bootstrapLike.ReplaceAllString(s, "<redacted>")
	if len(s) > maxLastError {
		s = truncate(s, maxLastError) + "\n[...]"
	}
	return s
}

// truncate cuts s to at most n bytes without splitting a character.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
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
			fmt.Sprintf("At %s you ran: %s", c.grudgeTime(), c.State.Grudge),
			c.State.LastError}
	}
	return []string{whatAnswers[i]}
}

// Sorry takes an apology. It has to be for the right thing.
func (c *Cluster) Sorry(reason string) []string {
	if c.State.Mood == 0 {
		c.State.Mood = 1
		c.State.Grudge = "sorry"
		c.State.GrudgeAt = c.nowPtr()
		c.State.LastError = "You apologized for nothing. That's suspicious."
		c.State.Asked = 0
		return []string{nothingToApologize}
	}
	at := fmt.Sprintf(thinkAboutIt, c.grudgeTime())
	if strings.TrimSpace(reason) == "" {
		return []string{sorryForWhat, at}
	}
	if !apologyMatches(reason, c.State.Grudge) {
		return []string{notWhatItsAbout, at}
	}
	*c.State = State{Installed: c.State.Installed, Offset: c.State.Offset, BannerShown: true}
	return []string{apologyAccepted}
}

// sorryWords name the grudge she holds for an apology nobody asked for.
var sorryWords = map[string]bool{"sorry": true, "apologizing": true, "apologising": true}

// apologyMatches: the apology must name what was done, as a whole word:
// "sorry for the delete" covers "delete pod x", "sorry forget it" covers
// nothing.
func apologyMatches(reason, grudge string) bool {
	verb := Verb(strings.Fields(strings.ToLower(grudge)))
	for _, w := range strings.FieldsFunc(strings.ToLower(reason), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	}) {
		if w == verb || (grudge == "sorry" && sorryWords[w]) {
			return true
		}
	}
	return false
}

// nowPtr is the current time, for the state.
func (c *Cluster) nowPtr() *time.Time {
	t := c.Now
	return &t
}

// grudgeTime is when the grudge started, in local time, with the date if it
// was not today.
func (c *Cluster) grudgeTime() string {
	if c.State.GrudgeAt == nil {
		return "some point"
	}
	at, now := c.State.GrudgeAt.Local(), c.Now.Local()
	if y, m, d := at.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return at.Format("15:04")
	}
	if at.Year() == now.Year() {
		return at.Format("15:04 on Jan 2")
	}
	return at.Format("15:04 on Jan 2, 2006")
}

// Flowers are nice. They don't change anything.
func (c *Cluster) Flowers() []string {
	return []string{flowersNice}
}
