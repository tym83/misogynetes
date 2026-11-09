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

// Package streak is when her trouble with a pod began, the same for the
// CLI and the webhook. A crash-looping pod is one continuous grievance: it
// is ready for a moment after every restart, and that moment is not the
// pod getting better. Only a container that stays up for Window has
// sorted itself out, and only trouble after that is new.
package streak

import "time"

// Window is how long a container must stay up before its trouble is over.
// The kubelet backs off a crash loop for at most five minutes, so a
// container that crashes within Window of starting is still in the same
// loop.
const Window = 5 * time.Minute

// Run is one finished run of a container.
type Run struct {
	Started, Finished time.Time
}

// backoff is the kubelet's crash-loop back-off before restart n (from 0):
// 10 seconds, doubling, at most five minutes.
func backoff(n int) time.Duration {
	d := 10 * time.Second
	for i := 0; i < n && d < Window; i++ {
		d *= 2
	}
	return min(d, Window)
}

// Start is when the trouble that ended last began, given the runs that
// came before it. A run of Window or longer was health: the trouble began
// when it ended. A shorter run is part of a crash loop that began before
// it: every earlier run is assumed as short as the last one and followed
// by the kubelet's back-off, so the estimate stays put while the loop
// goes on, however many restarts it adds.
func Start(last Run, before int) time.Time {
	run := last.Finished.Sub(last.Started)
	if run >= Window || run < 0 {
		return last.Finished
	}
	start := last.Started
	for i := 0; i < before; i++ {
		start = start.Add(-(backoff(i) + run))
	}
	return start
}
