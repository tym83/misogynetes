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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tym83/misogynetes/persona"
)

// Pod is the part of a Pod she reads: names, reasons and times, never a
// message, a log line or a field value.
type Pod struct {
	Metadata struct {
		Name              string     `json:"name"`
		DeletionTimestamp *time.Time `json:"deletionTimestamp,omitempty"`
	} `json:"metadata"`
	Status struct {
		Phase      string     `json:"phase"`
		Reason     string     `json:"reason"`
		StartTime  *time.Time `json:"startTime,omitempty"`
		Conditions []struct {
			Type               string    `json:"type"`
			Status             string    `json:"status"`
			LastTransitionTime time.Time `json:"lastTransitionTime"`
		} `json:"conditions"`
		ContainerStatuses []struct {
			State struct {
				Waiting *struct {
					Reason string `json:"reason"`
				} `json:"waiting,omitempty"`
				Terminated *struct {
					Reason string `json:"reason"`
				} `json:"terminated,omitempty"`
			} `json:"state"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

// Grievance is a pod in trouble she wants to be heard about.
type Grievance struct {
	Object string    // pod/web-1
	Why    string    // it's CrashLoopBackOff
	Since  time.Time // when the pod stopped being ready, as the pod says
}

// Grievances are the pods in trouble, in name order. Pods being deleted
// are not her concern any more.
func Grievances(pods []Pod) []Grievance {
	var out []Grievance
	for _, p := range pods {
		if p.Metadata.DeletionTimestamp != nil {
			continue
		}
		why := trouble(p)
		if why == "" {
			continue
		}
		g := Grievance{Object: "pod/" + p.Metadata.Name, Why: "it's " + why}
		if p.Status.StartTime != nil {
			g.Since = *p.Status.StartTime
		}
		for _, c := range p.Status.Conditions {
			if c.Type == "Ready" && c.Status != "True" && !c.LastTransitionTime.IsZero() {
				g.Since = c.LastTransitionTime
			}
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Object < out[j].Object })
	return out
}

// trouble is the reason a pod is in trouble, or "".
func trouble(p Pod) string {
	if p.Status.Phase == "Succeeded" {
		return ""
	}
	for _, c := range p.Status.ContainerStatuses {
		if w := c.State.Waiting; w != nil && persona.TroubleStatuses[w.Reason] {
			return w.Reason
		}
		if t := c.State.Terminated; t != nil && persona.TroubleStatuses[t.Reason] {
			return t.Reason
		}
	}
	if persona.TroubleStatuses[p.Status.Reason] {
		return p.Status.Reason
	}
	if p.Status.Phase == "Failed" {
		return "Failed"
	}
	return ""
}

// HeardAnnotation on a namespace is when someone last listened to her
// about it ("aga"): grievances older than that have been heard.
const HeardAnnotation = "misogynetes.dev/heard-at"

// Cluster is what she reads from the API server: the pods and the
// heard-at annotation of a namespace. Read-only.
type Cluster interface {
	Pods(ctx context.Context, namespace string) ([]Pod, error)
	HeardAt(ctx context.Context, namespace string) (time.Time, error)
}

// API reads the cluster with the pod's service account, with get and list
// on pods and get on namespaces, nothing else.
type API struct {
	Host      string
	TokenFile string
	Client    *http.Client
}

const saDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// InCluster is the API server of the cluster the webhook runs in.
func InCluster() (*API, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("not running in a cluster: KUBERNETES_SERVICE_HOST is not set")
	}
	ca, err := os.ReadFile(saDir + "/ca.crt")
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("no certificates in the service account CA")
	}
	return &API{
		Host:      "https://" + net.JoinHostPort(host, port),
		TokenFile: saDir + "/token",
		Client: &http.Client{
			Timeout:   2 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
	}, nil
}

func (a *API) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Host+path, nil)
	if err != nil {
		return err
	}
	if a.TokenFile != "" {
		// Read every time: projected tokens rotate.
		token, err := os.ReadFile(a.TokenFile)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	}
	req.Header.Set("Accept", "application/json")
	resp, err := a.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Pods lists the pods of a namespace, at most 500.
func (a *API) Pods(ctx context.Context, namespace string) ([]Pod, error) {
	var list struct {
		Items []Pod `json:"items"`
	}
	err := a.get(ctx, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/pods?limit=500", &list)
	return list.Items, err
}

// HeardAt is the heard-at annotation of a namespace; zero if none.
func (a *API) HeardAt(ctx context.Context, namespace string) (time.Time, error) {
	var ns struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}
	if err := a.get(ctx, "/api/v1/namespaces/"+url.PathEscape(namespace), &ns); err != nil {
		return time.Time{}, err
	}
	v := ns.Metadata.Annotations[HeardAnnotation]
	if v == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, v)
}

// cached keeps what she read about a namespace for a few seconds, so a
// burst of requests (an apply of many objects) reads the cluster once.
type cached struct {
	Cluster Cluster
	TTL     time.Duration

	mu      sync.Mutex
	entries map[string]entry
}

type entry struct {
	at      time.Time
	unheard []Grievance
	err     error
}

// unheard are the grievances in a namespace nobody listened to yet.
func (c *cached) unheard(ctx context.Context, namespace string, now time.Time) ([]Grievance, error) {
	c.mu.Lock()
	e, ok := c.entries[namespace]
	c.mu.Unlock()
	if ok && now.Sub(e.at) < c.TTL {
		return e.unheard, e.err
	}
	e = entry{at: now}
	pods, err := c.Cluster.Pods(ctx, namespace)
	if err == nil {
		var heard time.Time
		heard, err = c.Cluster.HeardAt(ctx, namespace)
		for _, g := range Grievances(pods) {
			if heard.IsZero() || g.Since.After(heard) {
				e.unheard = append(e.unheard, g)
			}
		}
	}
	e.err = err
	c.mu.Lock()
	if c.entries == nil || len(c.entries) > 1000 {
		c.entries = map[string]entry{}
	}
	c.entries[namespace] = e
	c.mu.Unlock()
	return e.unheard, err
}
