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

package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tym83/misogynetes/persona"
)

// BreakGlassAnnotation on the object being changed lets a fix through in
// enforce mode, whatever she says. Its value is the reason.
const BreakGlassAnnotation = "misogynetes.dev/break-glass"

// Misogynist is the webhook: she shares her grievances with every person
// who changes something in a namespace that opted in, and, in enforce
// mode only, refuses the change until somebody has listened.
type Misogynist struct {
	// ServiceAccounts makes service accounts (except those in
	// kube-system) hear her too. Off by default.
	ServiceAccounts bool
	// Off allows every request with nothing said.
	Off bool
	// Limiter keeps her from sharing the same thing with the same person
	// over and over. Refusals are never rate-limited.
	Limiter *Limiter
	Now     func() time.Time

	cluster *cached
	mu      sync.Mutex
	rand    *rand.Rand
}

// NewMisogynist reads the cluster through c, shares at most once per
// window per person and namespace, and caches what she read for a few
// seconds.
func NewMisogynist(c Cluster, window time.Duration, seed int64) *Misogynist {
	return &Misogynist{
		Limiter: &Limiter{Window: window},
		Now:     time.Now,
		cluster: &cached{Cluster: c, TTL: 10 * time.Second},
		rand:    rand.New(rand.NewSource(seed)),
	}
}

func (m *Misogynist) pick(lines []string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return lines[m.rand.Intn(len(lines))]
}

// Mode is how a namespace opted in.
type Mode string

const (
	// Warn shares grievances and never refuses anything.
	Warn Mode = "warn"
	// Enforce also refuses a change until she has been heard.
	Enforce Mode = "enforce"
)

// In is the decider for namespaces in mode.
func (m *Misogynist) In(mode Mode) Decider {
	return deciderFunc(func(r *Request) *Response { return m.Decide(mode, r) })
}

type deciderFunc func(*Request) *Response

func (f deciderFunc) Decide(r *Request) *Response { return f(r) }

// Decide decides one request in a namespace in mode.
func (m *Misogynist) Decide(mode Mode, r *Request) *Response {
	if m.Off || r.Namespace == "" || Untouchable(r) || !Human(r.UserInfo, m.ServiceAccounts) {
		return Allow()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	now := m.Now()
	unheard, err := m.cluster.unheard(ctx, r.Namespace, now)
	if err != nil {
		// She cannot see, so she has nothing to say. Never a reason to refuse.
		log.Printf("misogynetes-webhook: reading %s: %v", r.Namespace, err)
		return Allow()
	}
	if len(unheard) == 0 {
		return Allow()
	}
	share := m.share(unheard)
	if mode == Enforce {
		if reason := breakGlass(r); reason != "" {
			return Allow(persona.Insisted, fmt.Sprintf("(break-glass %q on %s: it went through. She is not done with you.)", reason, what(r)))
		}
		return &Response{Allowed: false, Result: &Status{
			Code: http.StatusForbidden,
			Message: fmt.Sprintf("%s %s Nothing was changed. Listen first: kubectl annotate namespace %s %s=$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ) --overwrite "+
				"(or, in an emergency, annotate the object %s=<reason>). Misogynetes is a satire of misogynists; the joke is on them.",
				m.pick(persona.HurtLines), share, r.Namespace, HeardAnnotation, BreakGlassAnnotation),
		}}
	}
	if dry := r.DryRun != nil && *r.DryRun; !dry && !m.Limiter.Allow(r.UserInfo.Username+"|"+r.Namespace, now) {
		return Allow()
	}
	return Allow(share, fmt.Sprintf("(she just wants to be heard: kubectl annotate namespace %s %s=<now>. Misogynetes, a satire of misogynists.)", r.Namespace, HeardAnnotation))
}

// share is one line about what she wants to be heard about.
func (m *Misogynist) share(gs []Grievance) string {
	line := strings.NewReplacer("{obj}", gs[0].Object, "{why}", gs[0].Why).Replace(m.pick(persona.ShareLines))
	if more := len(gs) - 1; more > 0 {
		line += fmt.Sprintf(persona.AndMore, more)
	}
	return line
}

// breakGlass is the reason given on the object being changed: on the new
// object for a create or an update, on the old one for a delete.
func breakGlass(r *Request) string {
	for _, raw := range []json.RawMessage{r.Object, r.OldObject} {
		var obj struct {
			Metadata struct {
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
		}
		if len(raw) > 0 && json.Unmarshal(raw, &obj) == nil {
			if v := strings.TrimSpace(obj.Metadata.Annotations[BreakGlassAnnotation]); v != "" {
				return v
			}
		}
		if r.Operation != "DELETE" {
			break
		}
	}
	return ""
}

func what(r *Request) string {
	s := strings.ToLower(r.Kind.Kind)
	if s == "" {
		s = r.Resource.Resource
	}
	if r.Name != "" {
		s += "/" + r.Name
	}
	return s
}
