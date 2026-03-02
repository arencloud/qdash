#!/usr/bin/env bash
set -euo pipefail

NS="${NS:-qdash-system}"
APP="${APP:-qdash}"
SERVICE="${SERVICE:-qdash}"
LOCAL_PORT="${LOCAL_PORT:-18080}"
SKIP_ROUTE_CHECK="${SKIP_ROUTE_CHECK:-false}"

log() {
  printf '[smoke] %s\n' "$*"
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

cleanup() {
  if [[ -n "${PF_PID:-}" ]] && kill -0 "$PF_PID" >/dev/null 2>&1; then
    kill "$PF_PID" >/dev/null 2>&1 || true
    wait "$PF_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

need_cmd kubectl
need_cmd curl

log "Checking rollout status for deploy/$APP in namespace $NS"
kubectl -n "$NS" rollout status "deploy/$APP" --timeout=180s

log "Checking ready pods for app=$APP"
kubectl -n "$NS" get pods -l "app=$APP" -o wide

log "Checking serviceaccount and cluster permissions"
kubectl -n "$NS" get sa "$APP" >/dev/null
kubectl auth can-i --as="system:serviceaccount:$NS:$APP" get gateways.gateway.networking.k8s.io -A >/dev/null
kubectl auth can-i --as="system:serviceaccount:$NS:$APP" create namespaces >/dev/null

log "Checking deployment security context"
kubectl -n "$NS" get deploy "$APP" -o jsonpath='{.spec.template.spec.securityContext.runAsNonRoot}{"\n"}' | grep -q '^true$'
kubectl -n "$NS" get deploy "$APP" -o jsonpath='{.spec.template.spec.containers[0].securityContext.allowPrivilegeEscalation}{"\n"}' | grep -q '^false$'

log "Checking probes are configured"
kubectl -n "$NS" get deploy "$APP" -o jsonpath='{.spec.template.spec.containers[0].readinessProbe.httpGet.path}{"\n"}' | grep -q '^/'
kubectl -n "$NS" get deploy "$APP" -o jsonpath='{.spec.template.spec.containers[0].livenessProbe.httpGet.path}{"\n"}' | grep -q '^/'

if [[ "$SKIP_ROUTE_CHECK" != "true" ]] && command -v oc >/dev/null 2>&1; then
  log "Checking OpenShift route (if present)"
  oc -n "$NS" get route "$APP" -o wide >/dev/null 2>&1 || log "Route $APP not found (continuing)"
fi

log "Port-forwarding service/$SERVICE to localhost:$LOCAL_PORT"
kubectl -n "$NS" port-forward "svc/$SERVICE" "$LOCAL_PORT:80" >/tmp/qdash-port-forward.log 2>&1 &
PF_PID=$!
sleep 3

log "Smoke checking /healthz"
curl -fsS "http://127.0.0.1:$LOCAL_PORT/healthz" >/dev/null

log "Smoke checking Swagger document"
curl -fsS "http://127.0.0.1:$LOCAL_PORT/swagger/doc.json" >/dev/null

log "Post-deploy smoke checks passed"
