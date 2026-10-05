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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandName(t *testing.T) {
	for argv0, want := range map[string]string{
		"misogynectl":                        "misogynectl",
		"/usr/local/bin/misogynetes":         "misogynectl",
		"/home/me/.krew/bin/kubectl-misogyn": "kubectl misogyn",
		"kubectl-misogyn.exe":                "kubectl misogyn",
		"kubectl-misogyn_more":               "kubectl misogyn-more",
		"kubectl-":                           "misogynectl",
	} {
		if got := commandName(argv0); got != want {
			t.Errorf("commandName(%q) = %q, want %q", argv0, got, want)
		}
	}
}

// TestAsKubectlPlugin runs her the way krew installs her: as the plugin
// kubectl-misogyn on PATH, called through the real kubectl. The real kubectl
// must not resolve back to the plugin, pipes must stay plain kubectl, exit
// codes honest, the frame intact, and hints must name the typed command.
func TestAsKubectlPlugin(t *testing.T) {
	kubectl, err := exec.LookPath("kubectl")
	if err != nil {
		t.Skip("kubectl not installed")
	}
	s := newSandbox(t, "echo \"kubectl ran: $*\"\nif [ \"$1\" = fail ] || [ \"$2\" = fail ]; then echo boom >&2; exit 3; fi\n")
	dir := t.TempDir()
	plugin := filepath.Join(dir, "kubectl-misogyn")
	if out, err := exec.Command("go", "build", "-o", plugin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	s.bin = kubectl
	path := "PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")

	want, err := exec.Command(kubectl, "version", "--client").Output()
	if err != nil {
		t.Fatalf("kubectl version --client: %v", err)
	}
	if out, errOut, code := s.run([]string{path, "MISOGYNETES_KUBECTL="}, "misogyn", "version", "--client"); code != 0 || errOut != "" || out != string(want) {
		t.Errorf("piped plugin is not plain kubectl: code %d stdout %q stderr %q", code, out, errOut)
	}
	_, errOut, code := s.run([]string{path}, "misogyn", "get", "fail")
	if code != 3 || !strings.Contains(errOut, "boom") {
		t.Errorf("piped failure not passed through: code %d stderr %q", code, errOut)
	}

	out, _, code := s.run([]string{path, "MISOGYNETES=always"}, "misogyn", "about")
	for _, want := range []string{"punishment for them", "https://github.com/tym83/kyvernetria", "`kubectl misogyn sorry for"} {
		if !strings.Contains(out, want) {
			t.Errorf("about via plugin without %q: %q", want, out)
		}
	}
	if code != 0 || strings.Contains(out, "misogynectl") {
		t.Errorf("about via plugin: code %d, names misogynectl: %q", code, out)
	}

	_, errOut, code = s.acting([]string{path}, "misogyn", "get", "fail")
	if code != 3 || !strings.Contains(errOut, "(kubectl exit 3 — `kubectl misogyn what`") {
		t.Errorf("hidden error via plugin: code %d stderr %q", code, errOut)
	}
}
