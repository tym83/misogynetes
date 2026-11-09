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

package streak

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// loop simulates the kubelet: the container runs for run, crashes, waits
// out the back-off and starts again, n times. It returns every finished
// run.
func loop(start time.Time, run time.Duration, n int) []Run {
	var runs []Run
	at := start
	for i := 0; i < n; i++ {
		runs = append(runs, Run{Started: at, Finished: at.Add(run)})
		at = at.Add(run + backoff(i))
	}
	return runs
}

func TestCrashLoopIsOneStreak(t *testing.T) {
	for _, run := range []time.Duration{0, 200 * time.Millisecond, 30 * time.Second, 4 * time.Minute} {
		runs := loop(t0, run, 40)
		for i, r := range runs {
			if got := Start(r, i); !got.Equal(t0) {
				t.Errorf("run %v, restart %d: streak began %v, want %v", run, i, got, t0)
			}
		}
	}
}

// The kubelet starts a container a little later than its back-off: the
// estimate drifts later by that slack per restart, never earlier than the
// loop began.
func TestSlackDriftsLate(t *testing.T) {
	at := t0
	for i := 0; i < 10; i++ {
		r := Run{Started: at, Finished: at.Add(time.Second)}
		got := Start(r, i)
		if got.Before(t0) || got.After(t0.Add(time.Duration(i)*2*time.Second)) {
			t.Errorf("restart %d: streak began %v", i, got)
		}
		at = at.Add(time.Second + backoff(i) + 2*time.Second)
	}
}

func TestLongRunEndsTheStreak(t *testing.T) {
	r := Run{Started: t0, Finished: t0.Add(Window + time.Second)}
	if got := Start(r, 7); !got.Equal(r.Finished) {
		t.Errorf("after a healthy run the trouble began at %v, want %v", got, r.Finished)
	}
}

func TestBackoff(t *testing.T) {
	want := []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second,
		160 * time.Second, Window, Window, Window}
	for i, w := range want {
		if got := backoff(i); got != w {
			t.Errorf("backoff(%d) = %v, want %v", i, got, w)
		}
	}
	if got := backoff(1 << 30); got != Window {
		t.Errorf("backoff(huge) = %v", got)
	}
}
