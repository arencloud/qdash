# QDash

Go 1.25 multi-tenant dashboard for GatewayAPI + Kuadrant policies using Gin, HTMX, GORM, and PostgreSQL.

## Key Features

- Multi-tenant organizations with bootstrap admin assignment.
- Built-in RBAC roles: `admin`, `editor`, `viewer` and custom permissions model.
- Service-account based Kubernetes/OpenShift control plane access (separate from user dashboard login).
- CRUD API for:
  - Gateway (`gateway.networking.k8s.io/v1`)
  - HTTPRoute (`gateway.networking.k8s.io/v1`)
  - AuthPolicy (`kuadrant.io/v1`)
  - RateLimitPolicy (`kuadrant.io/v1`)
- Namespace creation with selectable Istio label profiles.
- Namespace creation with selectable Istio instance + profile, plus org-level defaults.
- Swagger UI at `/swagger/index.html`.
- HTMX server-rendered dashboard pages.

## Run

```bash
cp .env.example .env # optional
make run
```

## Container Image (UBI Minimal, Rootless)

The project ships with a rootless runtime image based on `ubi9/ubi-minimal` (`Dockerfile`).

Build and push:

```bash
make image-build IMAGE=ghcr.io/<you>/qdash:dev CONTAINER_TOOL=podman
make image-push IMAGE=ghcr.io/<you>/qdash:dev CONTAINER_TOOL=podman
```

Version metadata is embedded at build time via ldflags:
- `VERSION` (default: `git describe --tags --always --dirty`)
- `COMMIT` (default: short git SHA)
- `BUILD_DATE` (default: UTC timestamp)

Example:

```bash
make build VERSION=0.2.0 COMMIT=$(git rev-parse --short HEAD) BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
```

## Swagger Generation

The runtime UI uses the curated static spec from `docs/docs.go`.

To generate an annotation-based OpenAPI artifact (JSON/YAML) without overwriting runtime docs:

```bash
make swagger-gen
```

This writes:
- `docs/generated/swagger.json`
- `docs/generated/swagger.yaml`

## Kubernetes Deployment

Base manifests are in `deploy/k8s/base`.

1. Create namespace:

```bash
kubectl apply -f deploy/k8s/base/namespace.yaml
```

2. Create runtime secret (recommended):

```bash
kubectl -n qdash-system create secret generic qdash-secret \
  --from-literal=DATABASE_URL='postgres://postgres:postgres@postgresql:5432/qdash?sslmode=disable' \
  --from-literal=OIDC_ISSUER_URL='https://issuer.example.com/realms/main' \
  --from-literal=OIDC_CLIENT_ID='qdash' \
  --from-literal=OIDC_CLIENT_SECRET='replace-me'
```

3. Apply manifests:

```bash
kubectl apply -k deploy/k8s/base
```

Optional template secret is provided at `deploy/k8s/base/secret.example.yaml`.

### OpenShift Overlay

OpenShift overlay is available in `deploy/k8s/overlays/openshift` and adds:
- `Route` resource
- SCC-compatible deployment security patch
- service account image pull secret wiring
- explicit rootless container runtime settings (`runAsNonRoot`, dropped capabilities, no privilege escalation)

1. Update host placeholders:
- `deploy/k8s/overlays/openshift/route.yaml` `spec.host`
- `deploy/k8s/overlays/openshift/patch-configmap.yaml` `OIDC_REDIRECT_URL`

2. Create image pull secret (if needed):

```bash
oc -n qdash-system create secret docker-registry qdash-pull-secret \
  --docker-server=ghcr.io \
  --docker-username=<username> \
  --docker-password=<token> \
  --docker-email=<email>
```

3. Apply overlay:

```bash
oc apply -k deploy/k8s/overlays/openshift
```

### OpenShift Dev Quickstart

Use the helper script to bootstrap/update a dev deployment in OpenShift:

```bash
export DATABASE_URL='postgres://postgres:postgres@postgresql:5432/qdash?sslmode=disable'
export OIDC_ISSUER_URL='https://issuer.example.com/realms/main'
export OIDC_CLIENT_ID='qdash'
export OIDC_CLIENT_SECRET='replace-me'

# Optional:
# export QDASH_NAMESPACE='qdash-system'
# export QDASH_ROUTE_HOST='qdash-dev.apps.<your-cluster-domain>'
# export QDASH_PULL_SECRET_NAME='qdash-pull-secret'
# export QDASH_IMAGE='ghcr.io/<you>/qdash:dev'

make openshift-dev-up
```

What it does:
- ensures namespace exists
- creates/updates `qdash-secret`
- applies OpenShift kustomize overlay
- patches Route host and `OIDC_REDIRECT_URL`
- restarts and waits for deployment rollout

## Environment

- `BIND_ADDRESS` default `:8080`
- `DATABASE_URL` default `postgres://postgres:postgres@localhost:5432/qdash?sslmode=disable`
- `KUBECONFIG` optional for local cluster access
- In-cluster mode uses pod service account automatically.
- OIDC browser login (required):
  - `OIDC_ISSUER_URL`
  - `OIDC_CLIENT_ID`
  - `OIDC_CLIENT_SECRET`
  - `OIDC_REDIRECT_URL` (example: `http://localhost:8080/auth/oidc/callback`)
  - `OIDC_SCOPES` default `openid,profile,email,groups`
 - On each successful OIDC login, enabled organization mappings are applied:
  - Group claim source is per-org `OIDCConfig.GroupClaim` (default `groups`)
  - Mappings support `subjectType` (`group`, `user`, `role`) + `externalValue`
  - Matching mappings grant/update org membership role and custom permission

## API

- Auth:
  - `GET /login`
  - `GET /auth/oidc/start`
  - `GET /auth/oidc/callback`
  - `POST /logout`
- `GET /api/v1/me`
- `GET|POST /api/v1/organizations`
- Org-scoped routes (`:orgSlug` required and enforced):
  - `GET /api/v1/orgs/:orgSlug/gatewayclasses`
  - `GET /api/v1/orgs/:orgSlug/istio-profiles`
  - `GET /api/v1/orgs/:orgSlug/istio-instances`
  - `GET /api/v1/orgs/:orgSlug/namespaces`
  - `POST /api/v1/orgs/:orgSlug/namespaces`
  - `POST /api/v1/orgs/:orgSlug/namespaces/adopt` (admin only; adopt pre-existing cluster namespace)
  - `GET|POST /api/v1/orgs/:orgSlug/gateways`
  - `GET|POST /api/v1/orgs/:orgSlug/httproutes`
  - `GET|POST /api/v1/orgs/:orgSlug/authpolicies`
  - `GET|POST /api/v1/orgs/:orgSlug/ratelimitpolicies`
  - `DELETE /api/v1/orgs/:orgSlug/{resource}/{namespace}/{name}`
  - `GET|PUT /api/v1/orgs/:orgSlug/oidc`
  - `GET|POST /api/v1/orgs/:orgSlug/oidc/mappings`
  - `DELETE /api/v1/orgs/:orgSlug/oidc/mappings/:mappingID`
  - `GET|POST /api/v1/orgs/:orgSlug/rbac/users`
  - `GET|POST /api/v1/orgs/:orgSlug/rbac/groups`
  - `DELETE /api/v1/orgs/:orgSlug/rbac/groups/:groupID`
  - `GET|POST /api/v1/orgs/:orgSlug/rbac/groups/:groupID/users`
  - `DELETE /api/v1/orgs/:orgSlug/rbac/groups/:groupID/users/:userID`
  - `GET|POST /api/v1/orgs/:orgSlug/rbac/groups/:groupID/permissions`
  - `DELETE /api/v1/orgs/:orgSlug/rbac/groups/:groupID/permissions/:permission`
  - `GET|POST /api/v1/orgs/:orgSlug/permissions`
  - `GET /api/v1/orgs/:orgSlug/audit-events?limit=100`

### API Quickstart (curl)

Swagger UI: `http://localhost:8080/swagger/index.html`

```bash
export BASE_URL="http://localhost:8080"
export ORG_SLUG="platform-team"
```

1. Start OIDC login in browser, then reuse session cookie in curl:

```bash
curl -i "$BASE_URL/api/v1/me" \
  -H "Cookie: qdash_session=<your-session-cookie>"
```

2. Create organization:

```bash
curl -sS -X POST "$BASE_URL/api/v1/organizations" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{"name":"Platform Team"}'
```

3. Create namespace ownership record + cluster namespace:

```bash
curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/namespaces" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{"name":"team-a","instance":"default","profile":"default","labels":["istio-injection=enabled"]}'
```

Namespace create precedence:
- Request payload `instance` / `profile` (if set)
- Organization settings `defaultNamespaceInstance` / `defaultNamespaceProfile`
- Hard fallback: `default` / `default`

4. Upsert Gateway:

```bash
curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/gateways" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{
    "namespace":"team-a",
    "name":"public-gateway",
    "spec":{
      "gatewayClassName":"openshift-default",
      "listeners":[{"name":"http","protocol":"HTTP","port":80}]
    }
  }'
```

5. Example semantic validation failure (`400` + `fieldErrors`):

```bash
curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/gateways" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{"namespace":"team-a","name":"bad-gw","spec":{"listeners":[{"name":"http","protocol":"HTTP","port":80}]}}'
```

Expected error shape:

```json
{
  "error": "semantic validation failed",
  "fieldErrors": [
    {"field": "spec.gatewayClassName", "message": "is required"}
  ]
}
```

6. Upsert HTTPRoute:

```bash
curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/httproutes" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{
    "namespace":"team-a",
    "name":"frontend-route",
    "spec":{
      "parentRefs":[{"group":"gateway.networking.k8s.io","kind":"Gateway","name":"public-gateway"}],
      "hostnames":["app.example.com"],
      "rules":[{"backendRefs":[{"name":"frontend-svc","port":8080}]}]
    }
  }'
```

7. Upsert AuthPolicy:

```bash
curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/authpolicies" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{
    "namespace":"team-a",
    "name":"frontend-authz",
    "spec":{
      "targetRef":{"group":"gateway.networking.k8s.io","kind":"HTTPRoute","name":"frontend-route"},
      "rules":{"authorization":{"allow":[{"when":[{"key":"request.headers[x-api-key]","operator":"eq","values":["demo-key"]}]}]}}
    }
  }'
```

8. Upsert RateLimitPolicy:

```bash
curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/ratelimitpolicies" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{
    "namespace":"team-a",
    "name":"frontend-ratelimit",
    "spec":{
      "targetRef":{"group":"gateway.networking.k8s.io","kind":"HTTPRoute","name":"frontend-route"},
      "limits":{"tenant-default":{"rates":[{"limit":100,"window":"1m"}]}}
    }
  }'
```

9. Delete resources:

```bash
curl -sS -X DELETE "$BASE_URL/api/v1/orgs/$ORG_SLUG/ratelimitpolicies/team-a/frontend-ratelimit" \
  -H "Cookie: qdash_session=<your-session-cookie>"

curl -sS -X DELETE "$BASE_URL/api/v1/orgs/$ORG_SLUG/authpolicies/team-a/frontend-authz" \
  -H "Cookie: qdash_session=<your-session-cookie>"

curl -sS -X DELETE "$BASE_URL/api/v1/orgs/$ORG_SLUG/httproutes/team-a/frontend-route" \
  -H "Cookie: qdash_session=<your-session-cookie>"

curl -sS -X DELETE "$BASE_URL/api/v1/orgs/$ORG_SLUG/gateways/team-a/public-gateway" \
  -H "Cookie: qdash_session=<your-session-cookie>"
```

10. Configure org OIDC integration:

```bash
curl -sS -X PUT "$BASE_URL/api/v1/orgs/$ORG_SLUG/oidc" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{
    "issuerUrl":"https://sso.example.com/realms/platform",
    "clientId":"qdash",
    "clientSecret":"replace-me",
    "groupClaim":"groups",
    "usernameClaim":"email",
    "enabled":true
  }'
```

11. Create OIDC mapping:

```bash
curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/oidc/mappings" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{
    "subjectType":"group",
    "externalValue":"platform-admins",
    "mappedRole":"admin",
    "customPermission":"security.approve"
  }'
```

Compatibility note: `externalGroup` is still accepted for legacy clients and is treated as `externalValue` when `subjectType=group`.

12. Create custom permission:

```bash
curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/permissions" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{
    "name":"security.approve",
    "resource":"security",
    "action":"approve",
    "definition":"Approve production security policy changes"
  }'
```

13. Upsert organization membership:

```bash
curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/rbac/users" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{
    "email":"alice@example.com",
    "role":"editor",
    "customPermissions":["gateway.write","security.read"]
  }'
```

14. Create group and assign permissions/members:

```bash
GROUP_ID=$(
  curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/rbac/groups" \
    -H "Content-Type: application/json" \
    -H "Cookie: qdash_session=<your-session-cookie>" \
    -d '{"name":"gateway-editors"}' | sed -n 's/.*"id":"\\([^"]*\\)".*/\\1/p'
)

curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/rbac/groups/$GROUP_ID/permissions" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{"permission":"gateway.write"}'

curl -sS -X POST "$BASE_URL/api/v1/orgs/$ORG_SLUG/rbac/groups/$GROUP_ID/users" \
  -H "Content-Type: application/json" \
  -H "Cookie: qdash_session=<your-session-cookie>" \
  -d '{"email":"alice@example.com"}'
```

15. Verify org security/rbac state:

```bash
curl -sS "$BASE_URL/api/v1/orgs/$ORG_SLUG/oidc" \
  -H "Cookie: qdash_session=<your-session-cookie>"

curl -sS "$BASE_URL/api/v1/orgs/$ORG_SLUG/oidc/mappings" \
  -H "Cookie: qdash_session=<your-session-cookie>"

curl -sS "$BASE_URL/api/v1/orgs/$ORG_SLUG/permissions" \
  -H "Cookie: qdash_session=<your-session-cookie>"

curl -sS "$BASE_URL/api/v1/orgs/$ORG_SLUG/rbac/users" \
  -H "Cookie: qdash_session=<your-session-cookie>"

curl -sS "$BASE_URL/api/v1/orgs/$ORG_SLUG/rbac/groups" \
  -H "Cookie: qdash_session=<your-session-cookie>"

curl -sS "$BASE_URL/api/v1/orgs/$ORG_SLUG/rbac/groups/$GROUP_ID/users" \
  -H "Cookie: qdash_session=<your-session-cookie>"

curl -sS "$BASE_URL/api/v1/orgs/$ORG_SLUG/rbac/groups/$GROUP_ID/permissions" \
  -H "Cookie: qdash_session=<your-session-cookie>"
```

16. Check recent audit events:

```bash
curl -sS "$BASE_URL/api/v1/orgs/$ORG_SLUG/audit-events?limit=20" \
  -H "Cookie: qdash_session=<your-session-cookie>"
```

## Troubleshooting

- `401 authentication required`
  - Cause: missing or expired `qdash_session` cookie.
  - Fix: run browser login again (`/auth/oidc/start`) and reuse the fresh cookie in curl.

- `403 forbidden` or `403 admin role required`
  - Cause: user has no matching org permission/role for endpoint.
  - Fix: check membership via `GET /api/v1/orgs/$ORG_SLUG/rbac/users`, then update with `POST /api/v1/orgs/$ORG_SLUG/rbac/users`.

- `404 organization not found or no membership`
  - Cause: wrong `orgSlug` or user not assigned to that organization.
  - Fix: confirm slug via `GET /api/v1/organizations`; ensure membership exists.

- `403 namespace is not owned by this organization`
  - Cause: resource apply/delete/list attempted in unclaimed namespace.
  - Fix: create namespace via `POST /api/v1/orgs/$ORG_SLUG/namespaces` or adopt existing namespace via `POST /api/v1/orgs/$ORG_SLUG/namespaces/adopt` (admin only).

- `400 semantic validation failed` with `fieldErrors`
  - Cause: resource `spec` fails server-side semantic checks (for example missing `gatewayClassName`, invalid ports, invalid rate window).
  - Fix: inspect `fieldErrors[].field` and `fieldErrors[].message`, adjust payload, retry.

- `502` from resource operations
  - Cause: Kubernetes API or CRD interaction failed from service-account context.
  - Fix: verify cluster connectivity, service account RBAC, and CRDs installation (`Gateway`, `HTTPRoute`, `AuthPolicy`, `RateLimitPolicy`).

## Production Hardening Checklist

- Transport security
  - Terminate TLS at ingress/route and enforce HTTPS redirects.
  - Set `OIDC_REDIRECT_URL` to an HTTPS callback URL in production.

- Session and cookie security
  - Ensure session cookies are `Secure`, `HttpOnly`, and scoped to the correct domain/path.
  - Use short session TTLs and require re-login on inactivity.

- Secrets and credentials
  - Store `DATABASE_URL`, `OIDC_CLIENT_SECRET`, and other credentials in Kubernetes/OpenShift Secrets, not ConfigMaps.
  - Rotate OIDC client secrets and DB credentials regularly.
  - Avoid committing real secret values to git history.

- Database security and reliability
  - Use PostgreSQL TLS (`sslmode=require` or stronger) for non-local environments.
  - Enable automated backups and verify restore procedures.
  - Set DB connection limits and monitor saturation.

- Kubernetes/OpenShift RBAC scope
  - Keep service account permissions least-privilege: only required verbs/resources for managed CRDs and namespaces.
  - Review and prune `ClusterRole` permissions periodically.

- Pod/container security
  - Keep rootless runtime settings enabled (`runAsNonRoot`, drop capabilities, no privilege escalation).
  - Use read-only root filesystem where possible.
  - Pin images by digest and scan images for CVEs in CI.

- Availability and scaling
  - Configure readiness/liveness probes and conservative startup thresholds.
  - Set CPU/memory requests and limits for predictable scheduling.
  - Run multiple replicas behind a stable Service for high availability.

- Observability and auditing
  - Forward application logs and Kubernetes events to centralized logging.
  - Monitor audit events (`/api/v1/orgs/:orgSlug/audit-events`) for privileged actions and failed operations.
  - Add alerting for repeated `401/403/502` spikes.

- API governance
  - Protect API and web endpoints with ingress rate limits/WAF where available.
  - Keep Swagger docs in sync with deployed behavior for operators and client SDKs.

### Repo Hardening Map

- TLS + external exposure
  - OpenShift Route host/TLS edge: [deploy/k8s/overlays/openshift/route.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/overlays/openshift/route.yaml)
  - OIDC callback URL config: [deploy/k8s/base/configmap.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/base/configmap.yaml), [deploy/k8s/overlays/openshift/patch-configmap.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/overlays/openshift/patch-configmap.yaml), [.env.example](/home/egevorky/Roo-Projects/qdash/.env.example)

- Secret handling
  - Runtime secret template: [deploy/k8s/base/secret.example.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/base/secret.example.yaml)
  - Deployment env secret wiring: [deploy/k8s/base/deployment.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/base/deployment.yaml)

- Service account + cluster permissions
  - Service account object: [deploy/k8s/base/serviceaccount.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/base/serviceaccount.yaml)
  - Cluster role scope: [deploy/k8s/base/clusterrole.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/base/clusterrole.yaml)
  - Binding to SA: [deploy/k8s/base/clusterrolebinding.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/base/clusterrolebinding.yaml)
  - OpenShift SA patch/image pull secret: [deploy/k8s/overlays/openshift/patch-serviceaccount.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/overlays/openshift/patch-serviceaccount.yaml)

- Rootless/container security
  - Baseline pod security context + container security context: [deploy/k8s/base/deployment.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/base/deployment.yaml)
  - OpenShift SCC-compatible security patch: [deploy/k8s/overlays/openshift/patch-deployment.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/overlays/openshift/patch-deployment.yaml)

- Availability and resource control
  - Replicas, probes, requests/limits: [deploy/k8s/base/deployment.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/base/deployment.yaml)
  - Overlay-specific deployment adjustments: [deploy/k8s/overlays/openshift/patch-deployment.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/overlays/openshift/patch-deployment.yaml)

- Overlay composition
  - OpenShift applied resources/patches: [deploy/k8s/overlays/openshift/kustomization.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/overlays/openshift/kustomization.yaml)

- App/runtime config defaults
  - Local env defaults: [.env.example](/home/egevorky/Roo-Projects/qdash/.env.example)
  - Core app configuration in cluster: [deploy/k8s/base/configmap.yaml](/home/egevorky/Roo-Projects/qdash/deploy/k8s/base/configmap.yaml)

### Pre-Deploy Verification Commands

Set target namespace:

```bash
export NS=qdash-system
```

1. Confirm manifests render as expected

```bash
kubectl kustomize deploy/k8s/base >/tmp/qdash-base.yaml
oc kustomize deploy/k8s/overlays/openshift >/tmp/qdash-ocp.yaml
```

2. Verify required secrets/config exist

```bash
kubectl -n "$NS" get secret qdash-secret
kubectl -n "$NS" get configmap qdash-config -o yaml
```

3. Verify service account and cluster permissions

```bash
kubectl -n "$NS" get sa qdash
kubectl get clusterrole qdash-cluster-role -o yaml
kubectl get clusterrolebinding qdash-cluster-rolebinding -o yaml
kubectl auth can-i --as=system:serviceaccount:$NS:qdash get gateways.gateway.networking.k8s.io -A
kubectl auth can-i --as=system:serviceaccount:$NS:qdash create namespaces
```

4. Verify rootless + security context on Deployment

```bash
kubectl -n "$NS" get deploy qdash -o jsonpath='{.spec.template.spec.securityContext.runAsNonRoot}{"\n"}'
kubectl -n "$NS" get deploy qdash -o jsonpath='{.spec.template.spec.containers[0].securityContext.allowPrivilegeEscalation}{"\n"}'
kubectl -n "$NS" get deploy qdash -o jsonpath='{.spec.template.spec.containers[0].securityContext.capabilities.drop}{"\n"}'
```

5. Verify probes and resource limits

```bash
kubectl -n "$NS" get deploy qdash -o jsonpath='{.spec.template.spec.containers[0].readinessProbe.httpGet.path}{"\n"}'
kubectl -n "$NS" get deploy qdash -o jsonpath='{.spec.template.spec.containers[0].livenessProbe.httpGet.path}{"\n"}'
kubectl -n "$NS" get deploy qdash -o jsonpath='{.spec.template.spec.containers[0].resources}{"\n"}'
```

6. Verify OpenShift route and host (OpenShift only)

```bash
oc -n "$NS" get route qdash -o wide
```

7. Verify rollout and runtime health

```bash
kubectl -n "$NS" rollout status deploy/qdash
kubectl -n "$NS" get pods -l app=qdash -o wide
kubectl -n "$NS" logs deploy/qdash --tail=200
```

8. Verify API and Swagger availability

```bash
kubectl -n "$NS" port-forward svc/qdash 8080:80
curl -sS http://127.0.0.1:8080/healthz
curl -sS http://127.0.0.1:8080/swagger/doc.json | head
```

Automated variant:

```bash
make smoke-post
```

Useful overrides:

```bash
NS=qdash-system APP=qdash SERVICE=qdash LOCAL_PORT=18080 make smoke-post
SKIP_ROUTE_CHECK=true make smoke-post
```

Web pages:
- `/organizations/:slug/audit` to review audit history (latest first).
- `/organizations/:slug/resources` HTMX CRUD for Gateway/HTTPRoute/AuthPolicy/RateLimitPolicy in owned namespaces.
  - Uses resource-specific form fields per kind.
  - Supports optional advanced JSON spec override for power users.
  - Includes namespace management panel for create/claim and admin-only adopt of existing namespaces.
  - Namespace creation uses selectable Istio profile labels from backend-supported profiles.
  - Supports row-level Edit: load existing resource into the form (fields + advanced JSON).

Namespace isolation rules:
- Every resource CRUD/list call requires `namespace`.
- Namespace must be owned by the organization in DB (`org_namespaces` table).
- Namespace ownership is established when creating namespace via `POST /api/v1/orgs/:orgSlug/namespaces`.
- Existing namespace adoption requires admin role and uses `POST /api/v1/orgs/:orgSlug/namespaces/adopt`.

Audit coverage:
- OIDC mapping decisions are logged.
- Namespace create/adopt and resource apply/delete actions are logged from both API and web flows.

## Notes

This foundation now includes OIDC browser login (auth code + PKCE + nonce), session auth, and strict per-org authorization checks. Next milestone should implement refresh-token/session rotation and organization-level OIDC role/group auto-sync on login.
