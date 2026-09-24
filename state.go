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
	"errors"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

// Update loads the state under a lock, lets change modify it and saves it,
// so that runs in several terminals at once never lose each other's
// changes. A state file that cannot be read is left alone: she stays calm
// for that run and nothing is written.
func Update(path string, now time.Time, r *rand.Rand, change func(*State)) {
	unlock := lockState(path)
	defer unlock()
	s, save := Load(path, now, r)
	change(s)
	if save {
		Save(path, s)
	}
}

// Load reads the state, creating it on first use with a random cycle
// offset. It reports whether the result may be saved back: not when the
// existing file is unreadable, since that would wipe it.
func Load(path string, now time.Time, r *rand.Rand) (*State, bool) {
	if path == "" {
		return &State{Installed: now, Offset: r.Intn(cycleDays)}, false
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &State{Installed: now, Offset: r.Intn(cycleDays)}, true
	}
	s := &State{}
	if err == nil && json.Unmarshal(b, s) == nil && !s.Installed.IsZero() {
		return s, true
	}
	return calm(now), false
}

// calm is the state used when the real one cannot be read: day 0 of the
// cycle, no mood, no banner.
func calm(now time.Time) *State {
	return &State{Installed: now, BannerShown: true}
}

// Save writes the state to a temporary file and renames it into place, so
// the file is always either the old state or the new one, never half.
func Save(path string, s *State) {
	if path == "" {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".state-*.json")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
	}
}
