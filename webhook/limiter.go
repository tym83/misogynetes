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
	"sync"
	"time"
)

// Limiter remembers what was said, so a reconcile loop or a re-apply does
// not hear the same thing again within Window.
type Limiter struct {
	Window time.Duration

	mu   sync.Mutex
	last map[string]time.Time
}

// maxKeys bounds the memory; past it, expired keys are dropped.
const maxKeys = 10000

// Allow reports whether key may be spoken about at now, and remembers it if
// so. A zero Window never limits.
func (l *Limiter) Allow(key string, now time.Time) bool {
	if l.Window <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.last == nil {
		l.last = map[string]time.Time{}
	}
	if t, ok := l.last[key]; ok && now.Sub(t) < l.Window {
		return false
	}
	if len(l.last) >= maxKeys {
		for k, t := range l.last {
			if now.Sub(t) >= l.Window {
				delete(l.last, k)
			}
		}
		if len(l.last) >= maxKeys {
			return false
		}
	}
	l.last[key] = now
	return true
}

// Seen reports whether key was spoken about within Window, without
// remembering anything.
func (l *Limiter) Seen(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t, ok := l.last[key]
	return ok && now.Sub(t) < l.Window
}
