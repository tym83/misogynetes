# Misogynetes chart

**A punishment for misogynists.**

Server mode for [Misogynetes](https://github.com/tym83/misogynetes): `kubectl`
that behaves exactly the way misogynists think women behave, now for every
client at once. If you find it accurate, that is the point, and it is on you.
What the evidence actually says about women, on average and with sources, is
in [Kyvernetria](https://github.com/tym83/kyvernetria).

This is "It's Not About the Nail" as a validating admission webhook. The nail
is a pod in trouble in the namespace (`CrashLoopBackOff`, `ImagePullBackOff`,
`OOMKilled`, `Error`, `Evicted`, ...). When a person changes something there,
she shares it, through the `Warning:` lines that kubectl, k9s, Lens and Argo CD
already show:

```text
$ kubectl -n shop set image deploy/web web=nginx:1.27
Warning: I just wanted to share: pod/web-6d4f8c9b7-xk2lp, it's CrashLoopBackOff. 🥺 (And 1 more. Don't ask.)
Warning: (she just wants to be heard: kubectl annotate namespace shop misogynetes.dev/heard-at=<now>. Misogynetes, a satire of misogynists.)
deployment.apps/web image updated
```

In a namespace in enforce mode she also refuses the fix until somebody has
listened:

```text
$ kubectl -n shop rollout restart deploy/web
Error from server (Forbidden): admission webhook "enforce.misogynetes.dev" denied the request:
You ALWAYS do this. You always try to fix it. 😤 I just wanted to share: pod/web-6d4f8c9b7-xk2lp, it's CrashLoopBackOff. 🥺
Nothing was changed. Listen first: kubectl annotate namespace shop misogynetes.dev/heard-at=$(date -u +%Y-%m-%dT%H:%M:%SZ) --overwrite
(or, in an emergency, annotate the object misogynetes.dev/break-glass=<reason>). ...

$ kubectl annotate namespace shop misogynetes.dev/heard-at=$(date -u +%Y-%m-%dT%H:%M:%SZ) --overwrite
$ kubectl -n shop rollout restart deploy/web
deployment.apps/web restarted
```

Grievances that started after the `heard-at` time (a pod that stopped being
ready later) are new, and she wants to be heard about them too. A crash loop
is one grievance from its first crash: the pod looks ready for a moment
after every restart, and that does not make it new. Only a pod that stayed
up for 5 minutes has sorted itself out, and trouble after that is new.

## Install

```bash
helm install misogynetes oci://ghcr.io/tym83/charts/misogynetes \
  --namespace misogynetes --create-namespace
kubectl label namespace shop misogynetes.dev/mode=warn       # shares only
kubectl label namespace lab  misogynetes.dev/mode=enforce    # also refuses fixes
```

Nothing happens until a namespace is labelled, and there is no cluster-wide
mode. Install it on your own cluster. Don't install it on anyone else's.

## What she never does

- Refuse anything outside a namespace labelled `misogynetes.dev/mode=enforce`.
  In `warn` namespaces every request is allowed.
- Refuse because she could not see: if the API server cannot be read, or the
  webhook is down or slow (`failurePolicy: Ignore`, 3 second timeout), the
  request goes through.
- Touch the cluster itself: nodes, controllers and every other `system:`
  identity pass untouched, as do service accounts (Argo CD, Flux, operators)
  unless `serviceAccounts=true`; those in `kube-system` always do. System
  namespaces (`kube-*` and the release namespace), nodes, leases, events and
  Secrets are never looked at.
- Write anything. Her service account can `get` and `list` pods and `get`
  namespaces, nothing else. She reads pod names, phases and reason words,
  never messages, logs or field values. The `heard-at` annotation is
  written by you.
- Repeat herself in warn mode: the same person hears about the same
  namespace at most once per `quietFor` (5 minutes). Refusals are not
  rate-limited; they are behaviour, not chatter.

The break-glass annotation (`misogynetes.dev/break-glass=<reason>`) is read
from the object being created or updated, and from the existing object for a
delete: annotate it first, then delete. `kubectl scale` sends a Scale, which
carries no annotations: patch `spec.replicas` on the annotated workload
instead. `kubectl exec` has no object to annotate; listen first.

## Values

| Value | Default | Meaning |
| --- | --- | --- |
| `namespaceLabel.key` | `misogynetes.dev/mode` | Label that opts a namespace in. |
| `namespaceLabel.warn` / `.enforce` | `warn` / `enforce` | Its values for the two modes. |
| `excludedNamespaces` | `kube-system`, `kube-public`, `kube-node-lease` | Never, labelled or not. The release namespace is always added. |
| `rules` | pods, services, configmaps, workloads and their `scale`, `pods/exec` | What counts as a fix. |
| `quietFor` | `5m` | Warn-mode rate limit per person and namespace. |
| `serviceAccounts` | `false` | Service accounts hear her too. |
| `off` | `false` | Silence her without uninstalling. |
| `certManager.enabled` | `false` | Use cert-manager for TLS; otherwise the chart generates a CA once and keeps it. |
| `image.repository` / `.tag` | `ghcr.io/tym83/misogynetes-webhook` / appVersion | Image. |
