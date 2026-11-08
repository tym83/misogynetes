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

// Package webhook is the server side of Misogynetes, a punishment for
// misogynists: a validating admission webhook that shares her grievances
// (pods in trouble in the namespace) as admission warnings, which every
// client shows. Only in namespaces that opted into enforce mode does she
// refuse a fix until she has been heard.
package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Review is the part of admission.k8s.io/v1 AdmissionReview the webhook
// reads and writes.
type Review struct {
	APIVersion string    `json:"apiVersion"`
	Kind       string    `json:"kind"`
	Request    *Request  `json:"request,omitempty"`
	Response   *Response `json:"response,omitempty"`
}

// Request is an AdmissionRequest.
type Request struct {
	UID         string          `json:"uid"`
	Kind        GroupKind       `json:"kind"`
	Resource    GroupResource   `json:"resource"`
	SubResource string          `json:"subResource,omitempty"`
	Name        string          `json:"name,omitempty"`
	Namespace   string          `json:"namespace,omitempty"`
	Operation   string          `json:"operation"`
	UserInfo    UserInfo        `json:"userInfo"`
	Object      json.RawMessage `json:"object,omitempty"`
	OldObject   json.RawMessage `json:"oldObject,omitempty"`
	DryRun      *bool           `json:"dryRun,omitempty"`
}

// GroupKind is the kind of the object in a request.
type GroupKind struct {
	Group string `json:"group"`
	Kind  string `json:"kind"`
}

// GroupResource is the resource of a request.
type GroupResource struct {
	Group    string `json:"group"`
	Resource string `json:"resource"`
}

// UserInfo is who sent the request.
type UserInfo struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups,omitempty"`
}

// Response is an AdmissionResponse.
type Response struct {
	UID      string   `json:"uid"`
	Allowed  bool     `json:"allowed"`
	Result   *Status  `json:"status,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Status explains a denial.
type Status struct {
	Code int32 `json:"code,omitempty"`
	// Reason is what kubectl names the error by: "Error from server
	// (Forbidden)". Without it the API server keeps the code but kubectl
	// prints a bare "Error from server".
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Decider decides one request.
type Decider interface {
	Decide(*Request) *Response
}

// maxBody bounds an AdmissionReview; the API server sends at most a few MB.
const maxBody = 8 << 20

// Handler serves AdmissionReviews with d.
func Handler(d Decider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST an AdmissionReview", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var review Review
		if err := json.Unmarshal(body, &review); err != nil || review.Request == nil {
			http.Error(w, fmt.Sprintf("not an AdmissionReview: %v", err), http.StatusBadRequest)
			return
		}
		resp := d.Decide(review.Request)
		resp.UID = review.Request.UID
		resp.Warnings = clip(resp.Warnings)
		out := Review{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview", Response: resp}
		if review.APIVersion != "" {
			out.APIVersion = review.APIVersion
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

// The API server may truncate warnings over 256 characters and drops
// everything past 4KB in total, so she keeps it short.
const (
	maxWarning  = 250
	maxWarnings = 8
)

func clip(ws []string) []string {
	if len(ws) > maxWarnings {
		ws = ws[:maxWarnings]
	}
	for i, w := range ws {
		w = strings.Map(func(r rune) rune {
			if r < ' ' || r == 0x7f {
				return ' '
			}
			return r
		}, w)
		if r := []rune(w); len(r) > maxWarning {
			w = string(r[:maxWarning-1]) + "…"
		}
		ws[i] = w
	}
	return ws
}

// Allow lets a request through untouched, with warnings if any.
func Allow(warnings ...string) *Response {
	return &Response{Allowed: true, Warnings: warnings}
}

// Human reports whether a request comes from a person rather than the
// cluster itself: nodes, controllers and other system: identities are
// never talked to or refused, and service accounts (Argo CD, Flux, operators) only
// when serviceAccounts is set, and never those in kube-system.
func Human(u UserInfo, serviceAccounts bool) bool {
	for _, g := range u.Groups {
		if g == "system:nodes" {
			return false
		}
	}
	if sa, ok := strings.CutPrefix(u.Username, "system:serviceaccount:"); ok {
		return serviceAccounts && !strings.HasPrefix(sa, "kube-system:")
	}
	return u.Username != "" && !strings.HasPrefix(u.Username, "system:")
}

// Untouchable reports requests she never comments on, whatever the webhook
// configuration sends: system namespaces, nodes, leases and events.
func Untouchable(r *Request) bool {
	if strings.HasPrefix(r.Namespace, "kube-") {
		return true
	}
	switch r.Resource.Resource {
	case "nodes", "leases", "events", "tokenreviews", "subjectaccessreviews", "secrets":
		return true
	}
	return r.Resource.Group == "coordination.k8s.io" || r.Resource.Group == "events.k8s.io"
}
