#!/usr/bin/env bash
set -euo pipefail

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_cmd oc

NS="${QDASH_NAMESPACE:-qdash-system}"
APP_NAME="${QDASH_APP_NAME:-qdash}"
SERVICE_NAME="${QDASH_SERVICE_NAME:-qdash}"
ROUTE_NAME="${QDASH_ROUTE_NAME:-qdash}"
IMAGE="${QDASH_IMAGE:-}"

DATABASE_URL="${DATABASE_URL:-}"
OIDC_ISSUER_URL="${OIDC_ISSUER_URL:-}"
OIDC_CLIENT_ID="${OIDC_CLIENT_ID:-}"
OIDC_CLIENT_SECRET="${OIDC_CLIENT_SECRET:-}"

if [[ -z "${DATABASE_URL}" || -z "${OIDC_ISSUER_URL}" || -z "${OIDC_CLIENT_ID}" || -z "${OIDC_CLIENT_SECRET}" ]]; then
  cat >&2 <<'EOF'
Required env vars are missing.
Set all of:
  DATABASE_URL
  OIDC_ISSUER_URL
  OIDC_CLIENT_ID
  OIDC_CLIENT_SECRET
EOF
  exit 1
fi

if [[ -z "${QDASH_ROUTE_HOST:-}" ]]; then
  CLUSTER_DOMAIN="$(oc get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}' 2>/dev/null || true)"
  if [[ -n "${CLUSTER_DOMAIN}" ]]; then
    QDASH_ROUTE_HOST="${APP_NAME}-${NS}.${CLUSTER_DOMAIN}"
  else
    echo "cannot detect OpenShift apps domain; set QDASH_ROUTE_HOST explicitly" >&2
    exit 1
  fi
fi

echo "Namespace: ${NS}"
echo "Route host: ${QDASH_ROUTE_HOST}"

oc get project "${NS}" >/dev/null 2>&1 || oc new-project "${NS}" >/dev/null

oc -n "${NS}" create secret generic qdash-secret \
  --from-literal=DATABASE_URL="${DATABASE_URL}" \
  --from-literal=OIDC_ISSUER_URL="${OIDC_ISSUER_URL}" \
  --from-literal=OIDC_CLIENT_ID="${OIDC_CLIENT_ID}" \
  --from-literal=OIDC_CLIENT_SECRET="${OIDC_CLIENT_SECRET}" \
  --dry-run=client -o yaml | oc apply -f -

oc apply -k deploy/k8s/overlays/openshift

if [[ -n "${QDASH_PULL_SECRET_NAME:-}" ]]; then
  echo "Using image pull secret: ${QDASH_PULL_SECRET_NAME}"
  oc -n "${NS}" patch serviceaccount qdash-sa --type=merge -p \
    '{"imagePullSecrets":[{"name":"'"${QDASH_PULL_SECRET_NAME}"'"}]}' >/dev/null
else
  # Overlay defaults to qdash-pull-secret; clear it for public images in dev.
  oc -n "${NS}" patch serviceaccount qdash-sa --type=merge -p \
    '{"imagePullSecrets":[]}' >/dev/null
fi

oc -n "${NS}" patch route "${ROUTE_NAME}" --type=merge -p \
  '{"spec":{"host":"'"${QDASH_ROUTE_HOST}"'"}}'

oc -n "${NS}" patch configmap qdash-config --type=merge -p \
  '{"data":{"OIDC_REDIRECT_URL":"https://'"${QDASH_ROUTE_HOST}"'/auth/oidc/callback"}}'

if [[ -n "${IMAGE}" ]]; then
  echo "Setting deployment image: ${IMAGE}"
  oc -n "${NS}" set image "deployment/${APP_NAME}" qdash="${IMAGE}" >/dev/null
fi

oc -n "${NS}" rollout restart "deployment/${APP_NAME}" >/dev/null
oc -n "${NS}" rollout status "deployment/${APP_NAME}" --timeout="${QDASH_ROLLOUT_TIMEOUT:-180s}"

echo
echo "QDash dev environment is ready:"
echo "  URL: https://${QDASH_ROUTE_HOST}"
echo "  Namespace: ${NS}"
