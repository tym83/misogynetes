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

// TestDeletedInOutputRealKubectl checks what she reads from a delete
// against the kubectl and cluster in CI: selectors, files and "-o name",
// with and without the "from <ns> namespace" kubectl adds. It needs a
// throwaway cluster and runs only with MISOGYNETES_E2E_KUBECTL=1
// (hack/e2e.sh sets it).
func TestDeletedInOutputRealKubectl(t *testing.T) {
	if os.Getenv("MISOGYNETES_E2E_KUBECTL") != "1" {
		t.Skip("set MISOGYNETES_E2E_KUBECTL=1 to run against a real cluster")
	}
	kubectl := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("kubectl", args...).Output()
		if err != nil {
			var stderr string
			if ee, ok := err.(*exec.ExitError); ok {
				stderr = string(ee.Stderr)
			}
			t.Fatalf("kubectl %v: %v: %s", args, err, stderr)
		}
		return string(out)
	}
	const ns = "misogynetes-delete-e2e"
	manifest := filepath.Join(t.TempDir(), "objects.yaml")
	objects := `apiVersion: v1
kind: ConfigMap
metadata: {name: cm-a, namespace: ` + ns + `, labels: {app: e2e}}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: cm-b, namespace: ` + ns + `, labels: {app: e2e}}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: ` + ns + `, labels: {app: e2e}}
spec:
  replicas: 0
  selector: {matchLabels: {app: web}}
  template:
    metadata: {labels: {app: web}}
    spec: {containers: [{name: web, image: registry.k8s.io/pause:3.10}]}
`
	if err := os.WriteFile(manifest, []byte(objects), 0o600); err != nil {
		t.Fatal(err)
	}
	kubectl("create", "namespace", ns)
	t.Cleanup(func() { _ = exec.Command("kubectl", "delete", "namespace", ns, "--wait=false").Run() })
	t.Logf("kubectl: %s", strings.TrimSpace(kubectl("version", "--client")))

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"selector", []string{"delete", "configmap,deployment", "-l", "app=e2e", "-n", ns},
			ns + "/configmap/cm-a " + ns + "/configmap/cm-b " + ns + "/deployment/web"},
		// No -n: the namespace comes from kubectl's own words.
		{"file", []string{"delete", "-f", manifest}, ns + "/configmap/cm-a " + ns + "/configmap/cm-b " + ns + "/deployment/web"},
		// "-o name" carries no namespace; she knows only what was typed.
		{"file -o name", []string{"delete", "-f", manifest, "-n", ns, "-o", "name"},
			ns + "/configmap/cm-a " + ns + "/configmap/cm-b " + ns + "/deployment/web"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kubectl("apply", "-f", manifest)
			out := kubectl(tc.args...)
			t.Logf("kubectl %s:\n%s", strings.Join(tc.args, " "), out)
			got := deletedInOutput(tc.args, out)
			if strings.Join(got, " ") != tc.want {
				t.Errorf("deletedInOutput = %q, want %q", got, tc.want)
			}
		})
	}
}
