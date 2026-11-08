#!/usr/bin/env bash
# End-to-end test of the misogynetes-webhook chart on a throwaway kind
# cluster (CI only: .github/workflows/e2e.yaml). It installs the chart with
# the image built from this tree, opts namespaces in and talks to the API
# server as people, controllers and service accounts.
#
#   CERT_MODE=helm|cert-manager IMAGE=misogynetes-webhook:e2e hack/e2e.sh
#
# Never point it at a cluster you care about: it labels namespaces, binds
# cluster-admin to a test user and scales the webhook down.
set -euo pipefail

CERT_MODE=${CERT_MODE:-helm}
IMAGE=${IMAGE:-misogynetes-webhook:e2e}
REL_NS=misogynetes
CHART=charts/misogynetes
WARN_NS=e2e-warn
ENFORCE_NS=e2e-enforce
QUIET_NS=e2e-quiet

failures=0
pass() { echo "PASS: $*"; }
fail() { echo "FAIL: $*"; failures=$((failures + 1)); }

# run captures a kubectl command: $out (stdout+stderr) and $code.
run() {
  set +e
  out=$("$@" 2>&1)
  code=$?
  set -e
  printf '$ %s\n%s\n' "$*" "$out" | sed 's/^/    /'
}
# as runs kubectl as a person: a user, not a service account.
as() { local who=$1; shift; run kubectl --as="$who" --as-group=system:authenticated "$@"; }
# as_sa runs kubectl as a service account.
as_sa() {
  local sa=$1; shift
  local ns=${sa#system:serviceaccount:}; ns=${ns%%:*}
  run kubectl --as="$sa" --as-group=system:serviceaccounts --as-group="system:serviceaccounts:$ns" --as-group=system:authenticated "$@"
}
expect_warning() { # what, pattern
  if [[ $code -eq 0 && $out == *"Warning:"* && $out == *"$2"* ]]; then pass "$1"; else fail "$1 (code $code)"; fi
}
expect_silent_ok() {
  if [[ $code -eq 0 && $out != *"Warning:"* ]]; then pass "$1"; else fail "$1 (code $code)"; fi
}
expect_ok() {
  if [[ $code -eq 0 ]]; then pass "$1"; else fail "$1 (code $code)"; fi
}
expect_forbidden() {
  if [[ $code -ne 0 && $out == *"Forbidden"* && $out == *'admission webhook "enforce.misogynetes.dev" denied the request'* && $out == *"pod/web-"* ]]; then
    pass "$1"
  else
    fail "$1 (code $code)"
  fi
}

# crashing waits until every web pod in $1 is in trouble (Error or
# CrashLoopBackOff) and none is starting.
crashing() {
  local ns=$1
  for _ in $(seq 90); do
    local reasons
    reasons=$(kubectl -n "$ns" get pods -l app=web -o jsonpath='{range .items[*]}{.status.containerStatuses[0].state.waiting.reason}{.status.containerStatuses[0].state.terminated.reason}{" "}{end}')
    if [[ -n ${reasons// /} ]] && ! grep -qvE '^(CrashLoopBackOff|Error)$' <<<"$(tr ' ' '\n' <<<"$reasons" | sed '/^$/d')"; then
      return 0
    fi
    sleep 2
  done
  kubectl -n "$ns" get pods -o wide
  echo "web pods in $ns never got into trouble" >&2
  exit 1
}

# She caches what she read about a namespace for 10 seconds.
forget_cache() { sleep 11; }

install() {
  local args=(--namespace "$REL_NS" --create-namespace --wait --timeout 3m
    --set image.repository="${IMAGE%:*}" --set image.tag="${IMAGE##*:}" --set image.pullPolicy=Never)
  if [[ $CERT_MODE == cert-manager ]]; then
    args+=(--set certManager.enabled=true)
  fi
  helm upgrade --install misogynetes "$CHART" "${args[@]}" "$@"
}

echo "== setup ($CERT_MODE)"
kubectl version
if [[ $CERT_MODE == cert-manager ]]; then
  helm repo add jetstack https://charts.jetstack.io --force-update >/dev/null
  helm install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace \
    --set crds.enabled=true --wait --timeout 5m
fi
install

if [[ $CERT_MODE == helm ]]; then
  echo "== the generated certificate survives helm upgrade"
  crt=$(kubectl -n "$REL_NS" get secret misogynetes-tls -o jsonpath='{.data.tls\.crt}')
  ca=$(kubectl get validatingwebhookconfiguration misogynetes -o jsonpath='{.webhooks[0].clientConfig.caBundle}')
  install
  crt2=$(kubectl -n "$REL_NS" get secret misogynetes-tls -o jsonpath='{.data.tls\.crt}')
  ca2=$(kubectl get validatingwebhookconfiguration misogynetes -o jsonpath='{.webhooks[0].clientConfig.caBundle}')
  if [[ -n $crt && $crt == "$crt2" && -n $ca && $ca == "$ca2" ]]; then pass "cert and caBundle reused across upgrade"; else fail "cert or caBundle regenerated on upgrade"; fi
else
  echo "== cert-manager injected the CA"
  for _ in $(seq 60); do
    ca=$(kubectl get validatingwebhookconfiguration misogynetes -o jsonpath='{.webhooks[1].clientConfig.caBundle}')
    [[ -n $ca ]] && break
    sleep 2
  done
  if [[ -n $ca ]]; then pass "caBundle injected by cert-manager"; else fail "no caBundle injected"; fi
fi

kubectl create clusterrolebinding e2e-people --clusterrole=cluster-admin --user=alice --user=bob --user=carol --user=dave
kubectl create namespace argocd
kubectl -n argocd create serviceaccount argocd-application-controller
# RBAC runs before admission: give the service accounts the rights, so it is
# her that lets them through.
kubectl create clusterrolebinding e2e-sa --clusterrole=cluster-admin \
  --serviceaccount=argocd:argocd-application-controller --serviceaccount=kube-system:deployment-controller

# Every namespace gets the same trouble before it opts in: a Deployment
# whose pod keeps crashing, and a calm pod.
for ns in "$WARN_NS" "$ENFORCE_NS" "$QUIET_NS"; do
  kubectl create namespace "$ns"
  kubectl -n "$ns" create deployment web --image=busybox:1.36 -- sh -c 'exit 1'
  kubectl -n "$ns" run calm --image=busybox:1.36 --restart=Never -- sleep 3600
  kubectl -n "$ns" create configmap note --from-literal=k=v
done
for ns in "$WARN_NS" "$ENFORCE_NS" "$QUIET_NS"; do
  crashing "$ns"
  kubectl -n "$ns" wait --for=condition=Ready pod/calm --timeout=3m
done
kubectl label namespace "$WARN_NS" misogynetes.dev/mode=warn
kubectl label namespace "$ENFORCE_NS" misogynetes.dev/mode=enforce

echo "== warn mode"
as alice -n "$WARN_NS" label configmap note touched=1 --overwrite
expect_warning "warn: a person hears her grievance" "pod/web-"
as alice -n "$WARN_NS" label configmap note touched=2 --overwrite
expect_silent_ok "warn: the same person is not told twice within quietFor"
as carol -n "$WARN_NS" rollout restart deployment/web
expect_warning "warn: rollout restart goes through with a warning" "pod/web-"
as dave -n "$WARN_NS" scale deployment/web --replicas=1
expect_warning "warn: scale goes through with a warning" "pod/web-"
as alice -n "$QUIET_NS" label configmap note touched=1 --overwrite
expect_silent_ok "unlabelled namespace: nothing said"
as_sa system:serviceaccount:kube-system:deployment-controller -n "$WARN_NS" label configmap note sa=1 --overwrite
expect_silent_ok "warn: kube-system controller hears nothing"
as_sa system:serviceaccount:argocd:argocd-application-controller -n "$WARN_NS" label configmap note argo=1 --overwrite
expect_silent_ok "warn: Argo-like service account hears nothing"

echo "== enforce mode"
as bob -n "$ENFORCE_NS" rollout restart deployment/web
expect_forbidden "enforce: rollout restart refused"
as bob -n "$ENFORCE_NS" scale deployment/web --replicas=2
expect_forbidden "enforce: scale refused"
as bob -n "$ENFORCE_NS" exec calm -- true
expect_forbidden "enforce: exec (CONNECT pods/exec) refused"
as bob -n "$ENFORCE_NS" delete pod calm --wait=false
expect_forbidden "enforce: delete refused"
as bob -n "$ENFORCE_NS" rollout restart deployment/web --dry-run=server
expect_forbidden "enforce: a server-side dry run is refused too"
as_sa system:serviceaccount:argocd:argocd-application-controller -n "$ENFORCE_NS" scale deployment/web --replicas=2
expect_silent_ok "enforce: Argo-like service account scales untouched"
for _ in $(seq 60); do
  [[ $(kubectl -n "$ENFORCE_NS" get pods -l app=web --no-headers 2>/dev/null | wc -l) -ge 2 ]] && break
  sleep 2
done
if [[ $(kubectl -n "$ENFORCE_NS" get pods -l app=web --no-headers | wc -l) -ge 2 ]]; then
  pass "enforce: replicaset controller created the pods"
else
  fail "enforce: replicaset controller did not create the pods"
fi
as_sa system:serviceaccount:argocd:argocd-application-controller -n "$ENFORCE_NS" scale deployment/web --replicas=1
expect_silent_ok "enforce: Argo-like service account scales back"

echo "== break-glass"
as bob -n "$ENFORCE_NS" annotate deployment web misogynetes.dev/break-glass=INC-42
expect_ok "break-glass: annotating the workload itself goes through"
as bob -n "$ENFORCE_NS" rollout restart deployment/web
expect_warning "break-glass: rollout restart goes through, with her last word" "break-glass"
# The deployment controller rolls out a new ReplicaSet and the replicaset
# controller its pod: neither is refused.
for _ in $(seq 60); do
  [[ $(kubectl -n "$ENFORCE_NS" get replicasets -l app=web --no-headers | wc -l) -ge 2 ]] && break
  sleep 2
done
if [[ $(kubectl -n "$ENFORCE_NS" get replicasets -l app=web --no-headers | wc -l) -ge 2 ]]; then
  pass "break-glass: deployment controller rolled out a new ReplicaSet"
else
  fail "break-glass: no new ReplicaSet"
fi
as bob -n "$ENFORCE_NS" scale deployment/web --replicas=2
expect_forbidden "break-glass: scale is still refused (a Scale carries no annotations)"
as bob -n "$ENFORCE_NS" annotate pod calm misogynetes.dev/break-glass=INC-43
expect_ok "break-glass: annotating the pod goes through"
as bob -n "$ENFORCE_NS" delete pod calm --wait=false
expect_warning "break-glass: delete of the annotated pod goes through" "break-glass"
as bob -n "$ENFORCE_NS" delete configmap note
expect_forbidden "break-glass: delete of an object without it is still refused"
kubectl -n "$ENFORCE_NS" annotate deployment web misogynetes.dev/break-glass- >/dev/null 2>&1 || true

echo "== heard-at lifts the refusal"
crashing "$ENFORCE_NS"
sleep 2
kubectl annotate namespace "$ENFORCE_NS" misogynetes.dev/heard-at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite
forget_cache
kubectl -n "$ENFORCE_NS" run calm2 --image=busybox:1.36 --restart=Never -- sleep 3600
kubectl -n "$ENFORCE_NS" wait --for=condition=Ready pod/calm2 --timeout=3m
as bob -n "$ENFORCE_NS" exec calm2 -- true
expect_ok "heard: exec goes through"
as bob -n "$ENFORCE_NS" scale deployment/web --replicas=2
expect_ok "heard: scale goes through"
as bob -n "$ENFORCE_NS" rollout restart deployment/web
expect_ok "heard: rollout restart goes through"

echo "== a grievance newer than heard-at is unheard again"
kubectl annotate namespace "$ENFORCE_NS" misogynetes.dev/heard-at="2000-01-01T00:00:00Z" --overwrite
forget_cache
as bob -n "$ENFORCE_NS" scale deployment/web --replicas=1
expect_forbidden "stale heard-at: refused again"

echo "== webhook down: requests pass (failurePolicy Ignore)"
kubectl -n "$REL_NS" scale deployment misogynetes --replicas=0
kubectl -n "$REL_NS" wait --for=delete pod -l app.kubernetes.io/name=misogynetes --timeout=2m || true
as bob -n "$ENFORCE_NS" scale deployment/web --replicas=1
expect_silent_ok "webhook down: enforce namespace lets the fix through"
kubectl -n "$REL_NS" scale deployment misogynetes --replicas=1

echo "== kubectl delete output as she parses it"
MISOGYNETES_E2E_KUBECTL=1 go test -run TestDeletedInOutputRealKubectl -count=1 -v . || fail "delete output parsing against the real kubectl"

if [[ $failures -gt 0 ]]; then
  echo "$failures check(s) failed"
  kubectl -n "$REL_NS" logs deployment/misogynetes --tail=100 || true
  exit 1
fi
echo "all checks passed"
