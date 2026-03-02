package docs

import "github.com/swaggo/swag"

var SwaggerInfo = &swag.Spec{
	Version:          "0.1.0",
	Host:             "localhost:8080",
	BasePath:         "/",
	Schemes:          []string{"http"},
	Title:            "QDash API",
	Description:      "Multi-tenant dashboard API for GatewayAPI, Kuadrant policies, and organization RBAC. Validation failures return structured fieldErrors.",
	InfoInstanceName: "swagger",
	SwaggerTemplate: `{
  "swagger": "2.0",
  "info": {
    "description": "Multi-tenant dashboard API for GatewayAPI and policy resources.",
    "title": "QDash API",
    "version": "0.1.0"
  },
  "host": "{{.Host}}",
  "basePath": "{{.BasePath}}",
  "tags": [
    {"name": "auth", "description": "Authentication and user identity"},
    {"name": "organizations", "description": "Organization lifecycle"},
    {"name": "gateway", "description": "GatewayAPI resources"},
    {"name": "security", "description": "Auth and rate limit policies"},
    {"name": "namespaces", "description": "Namespace ownership and provisioning"},
    {"name": "oidc", "description": "OIDC integration and mappings"},
    {"name": "rbac", "description": "Membership and permissions"},
    {"name": "audit", "description": "Audit trail"}
  ],
  "paths": {
    "/api/v1/me": {"get": {"operationId": "getCurrentUser", "tags": ["auth"], "summary": "Current user", "responses": {"200": {"description": "OK", "schema": {"$ref": "#/definitions/api.CurrentUserResponse"}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}}}},
    "/api/v1/organizations": {
      "get": {"operationId": "listOrganizations", "tags": ["organizations"], "summary": "List organizations", "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.OrganizationResponse"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}},
      "post": {"operationId": "createOrganization", "tags": ["organizations"], "summary": "Create organization", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.CreateOrganizationRequest"}}], "responses": {"201": {"description": "Created", "schema": {"$ref": "#/definitions/api.OrganizationResponse"}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "invalid character '}' looking for beginning of object key string"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}
    },
    "/api/v1/orgs/{orgSlug}/gateways": {"get": {"operationId": "listGateways", "tags": ["gateway"], "summary": "List gateways", "parameters": [{"in": "query", "name": "namespace", "required": true, "type": "string"}], "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.KubernetesResourceResponse"}}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}, "post": {"operationId": "upsertGateway", "tags": ["gateway"], "summary": "Upsert gateway", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.UpsertGatewayRequest"}}], "responses": {"200": {"description": "Applied", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "ok"}}}, "400": {"description": "Validation failed", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "semantic validation failed", "fieldErrors": [{"field": "spec.gatewayClassName", "message": "is required"}]}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}},
    "/api/v1/orgs/{orgSlug}/gateways/{namespace}/{name}": {"delete": {"operationId": "deleteGateway", "tags": ["gateway"], "summary": "Delete gateway", "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string", "default": "platform-team"}, {"in": "path", "name": "namespace", "required": true, "type": "string", "default": "team-a"}, {"in": "path", "name": "name", "required": true, "type": "string", "default": "public-gateway"}], "responses": {"200": {"description": "Deleted", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "deleted"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "NotFound"}}}}}},
    "/api/v1/orgs/{orgSlug}/httproutes": {"get": {"operationId": "listHTTPRoutes", "tags": ["gateway"], "summary": "List httproutes", "parameters": [{"in": "query", "name": "namespace", "required": true, "type": "string"}], "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.KubernetesResourceResponse"}}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}, "post": {"operationId": "upsertHTTPRoute", "tags": ["gateway"], "summary": "Upsert httproute", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.UpsertHTTPRouteRequest"}}], "responses": {"200": {"description": "Applied", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "ok"}}}, "400": {"description": "Validation failed", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "semantic validation failed", "fieldErrors": [{"field": "spec.rules", "message": "must contain at least one rule"}]}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}},
    "/api/v1/orgs/{orgSlug}/httproutes/{namespace}/{name}": {"delete": {"operationId": "deleteHTTPRoute", "tags": ["gateway"], "summary": "Delete httproute", "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string", "default": "platform-team"}, {"in": "path", "name": "namespace", "required": true, "type": "string", "default": "team-a"}, {"in": "path", "name": "name", "required": true, "type": "string", "default": "frontend-route"}], "responses": {"200": {"description": "Deleted", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "deleted"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "NotFound"}}}}}},
    "/api/v1/orgs/{orgSlug}/authpolicies": {"get": {"operationId": "listAuthPolicies", "tags": ["security"], "summary": "List authpolicies", "parameters": [{"in": "query", "name": "namespace", "required": true, "type": "string"}], "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.KubernetesResourceResponse"}}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}, "post": {"operationId": "upsertAuthPolicy", "tags": ["security"], "summary": "Upsert authpolicy", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.UpsertAuthPolicyRequest"}}], "responses": {"200": {"description": "Applied", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "ok"}}}, "400": {"description": "Validation failed", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "semantic validation failed", "fieldErrors": [{"field": "spec.targetRef.name", "message": "is required"}]}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}},
    "/api/v1/orgs/{orgSlug}/authpolicies/{namespace}/{name}": {"delete": {"operationId": "deleteAuthPolicy", "tags": ["security"], "summary": "Delete authpolicy", "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string", "default": "platform-team"}, {"in": "path", "name": "namespace", "required": true, "type": "string", "default": "team-a"}, {"in": "path", "name": "name", "required": true, "type": "string", "default": "frontend-authz"}], "responses": {"200": {"description": "Deleted", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "deleted"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "NotFound"}}}}}},
    "/api/v1/orgs/{orgSlug}/ratelimitpolicies": {"get": {"operationId": "listRateLimitPolicies", "tags": ["security"], "summary": "List ratelimitpolicies", "parameters": [{"in": "query", "name": "namespace", "required": true, "type": "string"}], "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.KubernetesResourceResponse"}}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}, "post": {"operationId": "upsertRateLimitPolicy", "tags": ["security"], "summary": "Upsert ratelimitpolicy", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.UpsertRateLimitPolicyRequest"}}], "responses": {"200": {"description": "Applied", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "ok"}}}, "400": {"description": "Validation failed", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "semantic validation failed", "fieldErrors": [{"field": "spec.limits.tenant-default.rates[0].window", "message": "must match rate window format like 1m, 10s, 1h30m"}]}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}},
    "/api/v1/orgs/{orgSlug}/ratelimitpolicies/{namespace}/{name}": {"delete": {"operationId": "deleteRateLimitPolicy", "tags": ["security"], "summary": "Delete ratelimitpolicy", "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string", "default": "platform-team"}, {"in": "path", "name": "namespace", "required": true, "type": "string", "default": "team-a"}, {"in": "path", "name": "name", "required": true, "type": "string", "default": "frontend-ratelimit"}], "responses": {"200": {"description": "Deleted", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "deleted"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "NotFound"}}}}}},
    "/api/v1/orgs/{orgSlug}/gatewayclasses": {"get": {"operationId": "listGatewayClasses", "tags": ["gateway"], "summary": "List gateway classes", "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"type": "string"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}},
    "/api/v1/orgs/{orgSlug}/istio-profiles": {"get": {"operationId": "listIstioProfiles", "tags": ["namespaces"], "summary": "List available Istio namespace profiles", "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"type": "string"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}},
    "/api/v1/orgs/{orgSlug}/istio-instances": {"get": {"operationId": "listIstioInstances", "tags": ["namespaces"], "summary": "List available Istio instances", "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"type": "string"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}},
    "/api/v1/orgs/{orgSlug}/namespaces": {"get": {"operationId": "listNamespaces", "tags": ["namespaces"], "summary": "List owned namespaces", "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.NamespaceStatusResponse"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}, "post": {"operationId": "createNamespace", "tags": ["namespaces"], "summary": "Create namespace with Istio labels", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.CreateNamespaceRequest"}}], "responses": {"201": {"description": "Created", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "created"}}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "Key: 'CreateNamespaceRequest.Name' Error:Field validation for 'Name' failed on the 'required' tag"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}, "409": {"description": "Namespace already owned", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "namespace already owned by another organization"}}}}}},
    "/api/v1/orgs/{orgSlug}/namespaces/adopt": {"post": {"operationId": "adoptNamespace", "tags": ["namespaces"], "summary": "Adopt existing namespace (admin only)", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.AdoptNamespaceRequest"}}], "responses": {"201": {"description": "Created", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "adopted"}}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "namespace name is required"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "admin role required"}}}, "404": {"description": "Namespace not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "namespace not found in cluster"}}}, "409": {"description": "Namespace already owned", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "namespace already owned by another organization"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "502": {"description": "Bad gateway", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "cluster api unavailable"}}}}}},
    "/api/v1/orgs/{orgSlug}/oidc": {"get": {"operationId": "getOIDCConfig", "tags": ["oidc"], "summary": "Get OIDC config", "responses": {"200": {"description": "OK", "schema": {"$ref": "#/definitions/api.OIDCConfigResponse"}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}, "put": {"operationId": "upsertOIDCConfig", "tags": ["oidc"], "summary": "Upsert OIDC config", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.UpsertOIDCConfigRequest"}}], "responses": {"200": {"description": "Updated", "schema": {"$ref": "#/definitions/api.StatusResponse"}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "invalid character '}' looking for beginning of object key string"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}}},
    "/api/v1/orgs/{orgSlug}/oidc/mappings": {"get": {"operationId": "listOIDCMappings", "tags": ["oidc"], "summary": "List OIDC mappings", "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.OIDCMappingResponse"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}, "post": {"operationId": "createOIDCMapping", "tags": ["oidc"], "summary": "Create OIDC mapping", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.CreateOIDCMappingRequest"}}], "responses": {"201": {"description": "Created", "schema": {"$ref": "#/definitions/api.StatusResponse"}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "invalid character '}' looking for beginning of object key string"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}}},
    "/api/v1/orgs/{orgSlug}/oidc/mappings/{mappingID}": {"delete": {"operationId": "deleteOIDCMapping", "tags": ["oidc"], "summary": "Delete OIDC mapping", "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string", "default": "platform-team"}, {"in": "path", "name": "mappingID", "required": true, "type": "string", "format": "uuid", "default": "f61f9b8a-4f6c-4e4c-b181-e330f9c67a85"}], "responses": {"200": {"description": "Deleted", "schema": {"$ref": "#/definitions/api.StatusResponse"}, "examples": {"application/json": {"status": "deleted"}}}, "400": {"description": "Invalid mapping id", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "invalid mapping id"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "record not found"}}}}}},
    "/api/v1/orgs/{orgSlug}/rbac/users": {"get": {"operationId": "listMemberships", "tags": ["rbac"], "summary": "List memberships", "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.MembershipResponse"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}, "post": {"operationId": "upsertMembership", "tags": ["rbac"], "summary": "Upsert membership", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.UpsertMembershipRequest"}}], "responses": {"200": {"description": "Updated", "schema": {"$ref": "#/definitions/api.StatusResponse"}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "invalid character '}' looking for beginning of object key string"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}}},
    "/api/v1/orgs/{orgSlug}/rbac/groups": {"get": {"operationId": "listGroups", "tags": ["rbac"], "summary": "List groups", "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.GroupResponse"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}, "post": {"operationId": "createGroup", "tags": ["rbac"], "summary": "Create group", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.CreateGroupRequest"}}], "responses": {"201": {"description": "Created", "schema": {"$ref": "#/definitions/api.GroupResponse"}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}},
    "/api/v1/orgs/{orgSlug}/rbac/groups/{groupID}": {"delete": {"operationId": "deleteGroup", "tags": ["rbac"], "summary": "Delete group", "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string"}, {"in": "path", "name": "groupID", "required": true, "type": "string", "format": "uuid"}], "responses": {"200": {"description": "Deleted", "schema": {"$ref": "#/definitions/api.StatusResponse"}}, "400": {"description": "Invalid id", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "group not found"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}},
    "/api/v1/orgs/{orgSlug}/rbac/groups/{groupID}/users": {"get": {"operationId": "listGroupMembers", "tags": ["rbac"], "summary": "List group members", "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string"}, {"in": "path", "name": "groupID", "required": true, "type": "string", "format": "uuid"}], "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.GroupMemberResponse"}}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "group not found"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}, "post": {"operationId": "addGroupMember", "tags": ["rbac"], "summary": "Add group member", "consumes": ["application/json"], "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string"}, {"in": "path", "name": "groupID", "required": true, "type": "string", "format": "uuid"}, {"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.AddGroupMemberRequest"}}], "responses": {"201": {"description": "Created", "schema": {"$ref": "#/definitions/api.GroupMemberResponse"}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "group not found"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}},
    "/api/v1/orgs/{orgSlug}/rbac/groups/{groupID}/users/{userID}": {"delete": {"operationId": "removeGroupMember", "tags": ["rbac"], "summary": "Remove group member", "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string"}, {"in": "path", "name": "groupID", "required": true, "type": "string", "format": "uuid"}, {"in": "path", "name": "userID", "required": true, "type": "string", "format": "uuid"}], "responses": {"200": {"description": "Deleted", "schema": {"$ref": "#/definitions/api.StatusResponse"}}, "400": {"description": "Invalid id", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "group not found"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}},
    "/api/v1/orgs/{orgSlug}/rbac/groups/{groupID}/permissions": {"get": {"operationId": "listGroupPermissions", "tags": ["rbac"], "summary": "List group permissions", "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string"}, {"in": "path", "name": "groupID", "required": true, "type": "string", "format": "uuid"}], "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.GroupPermissionResponse"}}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "group not found"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}, "post": {"operationId": "addGroupPermission", "tags": ["rbac"], "summary": "Add group permission", "consumes": ["application/json"], "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string"}, {"in": "path", "name": "groupID", "required": true, "type": "string", "format": "uuid"}, {"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.AddGroupPermissionRequest"}}], "responses": {"201": {"description": "Created", "schema": {"$ref": "#/definitions/api.GroupPermissionResponse"}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "group not found"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}},
    "/api/v1/orgs/{orgSlug}/rbac/groups/{groupID}/permissions/{permission}": {"delete": {"operationId": "removeGroupPermission", "tags": ["rbac"], "summary": "Remove group permission", "parameters": [{"in": "path", "name": "orgSlug", "required": true, "type": "string"}, {"in": "path", "name": "groupID", "required": true, "type": "string", "format": "uuid"}, {"in": "path", "name": "permission", "required": true, "type": "string"}], "responses": {"200": {"description": "Deleted", "schema": {"$ref": "#/definitions/api.StatusResponse"}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "404": {"description": "Not found", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "group not found"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}},
    "/api/v1/orgs/{orgSlug}/permissions": {"get": {"operationId": "listPermissions", "tags": ["rbac"], "summary": "List permissions", "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.PermissionResponse"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}, "post": {"operationId": "createPermission", "tags": ["rbac"], "summary": "Create permission", "consumes": ["application/json"], "parameters": [{"in": "body", "name": "request", "required": true, "schema": {"$ref": "#/definitions/api.CreatePermissionRequest"}}], "responses": {"201": {"description": "Created", "schema": {"$ref": "#/definitions/api.StatusResponse"}}, "400": {"description": "Invalid request", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "invalid character '}' looking for beginning of object key string"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "authentication required"}}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "forbidden"}}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}}},
    "/api/v1/orgs/{orgSlug}/audit-events": {"get": {"operationId": "listAuditEvents", "tags": ["audit"], "summary": "List audit events", "parameters": [{"in": "query", "name": "limit", "type": "integer"}, {"in": "query", "name": "resource", "type": "string"}, {"in": "query", "name": "status", "type": "string"}, {"in": "query", "name": "eventType", "type": "string"}], "responses": {"200": {"description": "OK", "schema": {"type": "array", "items": {"$ref": "#/definitions/api.AuditEventResponse"}}}, "401": {"description": "Unauthorized", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "403": {"description": "Forbidden", "schema": {"$ref": "#/definitions/api.ErrorResponse"}}, "500": {"description": "Internal server error", "schema": {"$ref": "#/definitions/api.ErrorResponse"}, "examples": {"application/json": {"error": "internal server error"}}}}}}
  },
  "definitions": {
    "api.CurrentUserResponse": {
      "type": "object",
      "properties": {
        "id": {"type": "string", "format": "uuid", "example": "f1a4653b-c87d-4253-954b-312769b3b018"},
        "email": {"type": "string", "example": "admin@example.com"},
        "displayName": {"type": "string", "example": "Platform Admin"},
        "source": {"type": "string", "example": "oidc"}
      }
    },
    "api.StatusResponse": {
      "type": "object",
      "properties": {
        "status": {"type": "string", "example": "ok"}
      }
    },
    "api.KubernetesResourceResponse": {
      "type": "object",
      "properties": {
        "apiVersion": {"type": "string", "example": "gateway.networking.k8s.io/v1"},
        "kind": {"type": "string", "example": "Gateway"},
        "metadata": {"type": "object", "additionalProperties": true},
        "spec": {"type": "object", "additionalProperties": true}
      }
    },
    "api.NamespaceStatusResponse": {
      "type": "object",
      "properties": {
        "name": {"type": "string", "example": "team-a"},
        "exists": {"type": "boolean", "example": true}
      }
    },
    "api.AuditEventResponse": {
      "type": "object",
      "properties": {
        "id": {"type": "string", "format": "uuid", "example": "f61f9b8a-4f6c-4e4c-b181-e330f9c67a85"},
        "createdAt": {"type": "string", "format": "date-time", "example": "2026-03-02T09:01:02Z"},
        "eventType": {"type": "string", "example": "resource.apply"},
        "resource": {"type": "string", "example": "gateways"},
        "status": {"type": "string", "example": "success"},
        "message": {"type": "string", "example": "resource applied"},
        "actorId": {"type": "string", "format": "uuid", "example": "2f9d4e9e-7dc5-4f98-beb4-eb677dbf41ec"},
        "details": {"type": "object", "additionalProperties": true}
      }
    },
    "api.OrganizationResponse": {
      "type": "object",
      "properties": {
        "id": {"type": "string", "format": "uuid", "example": "2f9d4e9e-7dc5-4f98-beb4-eb677dbf41ec"},
        "name": {"type": "string", "example": "Platform Team"},
        "slug": {"type": "string", "example": "platform-team"}
      }
    },
    "api.OIDCConfigResponse": {
      "type": "object",
      "properties": {
        "id": {"type": "string", "format": "uuid", "example": "f1a4653b-c87d-4253-954b-312769b3b018"},
        "issuerUrl": {"type": "string", "example": "https://sso.example.com/realms/platform"},
        "clientId": {"type": "string", "example": "qdash"},
        "groupClaim": {"type": "string", "example": "groups"},
        "usernameClaim": {"type": "string", "example": "email"},
        "enabled": {"type": "boolean", "example": true}
      }
    },
    "api.OIDCMappingResponse": {
      "type": "object",
      "properties": {
        "id": {"type": "string", "format": "uuid", "example": "f61f9b8a-4f6c-4e4c-b181-e330f9c67a85"},
        "subjectType": {"type": "string", "example": "group"},
        "externalValue": {"type": "string", "example": "platform-admins"},
        "externalGroup": {"type": "string", "example": "platform-admins"},
        "mappedRole": {"type": "string", "example": "admin"},
        "customPermission": {"type": "string", "example": "security.approve"}
      }
    },
    "api.MembershipResponse": {
      "type": "object",
      "properties": {
        "id": {"type": "string", "format": "uuid", "example": "2f1e240f-f6bc-4f26-b275-f42bda6fbefe"},
        "role": {"type": "string", "example": "editor"},
        "permission": {"type": "string", "example": "gateway.write,security.read"}
      }
    },
    "api.GroupResponse": {
      "type": "object",
      "properties": {
        "id": {"type": "string", "format": "uuid", "example": "7a1f240f-f6bc-4f26-b275-f42bda6fb111"},
        "name": {"type": "string", "example": "gateway-editors"}
      }
    },
    "api.GroupMemberResponse": {
      "type": "object",
      "properties": {
        "id": {"type": "string", "format": "uuid", "example": "2f1e240f-f6bc-4f26-b275-f42bda6fbefe"},
        "email": {"type": "string", "example": "alice@example.com"},
        "displayName": {"type": "string", "example": "alice"}
      }
    },
    "api.GroupPermissionResponse": {
      "type": "object",
      "properties": {
        "permission": {"type": "string", "example": "gateway.write"}
      }
    },
    "api.PermissionResponse": {
      "type": "object",
      "properties": {
        "id": {"type": "string", "format": "uuid", "example": "10c59a47-3df8-4f13-9ce3-2354cd42039c"},
        "name": {"type": "string", "example": "security.approve"},
        "resource": {"type": "string", "example": "security"},
        "action": {"type": "string", "example": "approve"},
        "definition": {"type": "string", "example": "Approve production security policy changes"},
        "isBuiltIn": {"type": "boolean", "example": false}
      }
    },
    "api.CreateOrganizationRequest": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": {"type": "string", "example": "Platform Team"}
      }
    },
    "api.CreateNamespaceRequest": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": {"type": "string", "example": "team-a"},
        "instance": {"type": "string", "example": "default"},
        "profile": {"type": "string", "example": "default"},
        "labels": {"type": "array", "items": {"type": "string"}, "example": ["istio-injection=enabled", "istio.io/rev=default"]}
      }
    },
    "api.AdoptNamespaceRequest": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": {"type": "string", "example": "shared-gateway-ns"}
      }
    },
    "api.UpsertOIDCConfigRequest": {
      "type": "object",
      "required": ["issuerUrl", "clientId", "clientSecret"],
      "properties": {
        "issuerUrl": {"type": "string", "example": "https://sso.example.com/realms/platform"},
        "clientId": {"type": "string", "example": "qdash"},
        "clientSecret": {"type": "string", "example": "change-me"},
        "groupClaim": {"type": "string", "example": "groups"},
        "usernameClaim": {"type": "string", "example": "preferred_username"},
        "enabled": {"type": "boolean", "example": true}
      }
    },
    "api.CreateOIDCMappingRequest": {
      "type": "object",
      "required": ["externalValue", "mappedRole"],
      "properties": {
        "subjectType": {"type": "string", "example": "group"},
        "externalValue": {"type": "string", "example": "platform-admins"},
        "externalGroup": {"type": "string", "example": "platform-admins"},
        "mappedRole": {"type": "string", "example": "admin"},
        "customPermission": {"type": "string", "example": "security.approve"}
      }
    },
    "api.UpsertMembershipRequest": {
      "type": "object",
      "required": ["email", "role"],
      "properties": {
        "email": {"type": "string", "example": "alice@example.com"},
        "role": {"type": "string", "example": "editor"},
        "customPermissions": {"type": "array", "items": {"type": "string"}, "example": ["gateway.write", "security.read"]}
      }
    },
    "api.CreateGroupRequest": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": {"type": "string", "example": "gateway-editors"}
      }
    },
    "api.AddGroupMemberRequest": {
      "type": "object",
      "required": ["email"],
      "properties": {
        "email": {"type": "string", "example": "alice@example.com"}
      }
    },
    "api.AddGroupPermissionRequest": {
      "type": "object",
      "required": ["permission"],
      "properties": {
        "permission": {"type": "string", "example": "gateway.write"}
      }
    },
    "api.CreatePermissionRequest": {
      "type": "object",
      "required": ["name", "resource", "action"],
      "properties": {
        "name": {"type": "string", "example": "security.approve"},
        "resource": {"type": "string", "example": "security"},
        "action": {"type": "string", "example": "approve"},
        "definition": {"type": "string", "example": "Approve production security policy changes"}
      }
    },
    "api.UpsertGatewayRequest": {
      "type": "object",
      "required": ["namespace", "name", "spec"],
      "properties": {
        "namespace": {"type": "string", "example": "team-a"},
        "name": {"type": "string", "example": "public-gateway"},
        "spec": {"type": "object", "example": {"gatewayClassName": "openshift-default", "listeners": [{"name": "http", "protocol": "HTTP", "port": 80}]}}
      }
    },
    "api.UpsertHTTPRouteRequest": {
      "type": "object",
      "required": ["namespace", "name", "spec"],
      "properties": {
        "namespace": {"type": "string", "example": "team-a"},
        "name": {"type": "string", "example": "frontend-route"},
        "spec": {"type": "object", "example": {"parentRefs": [{"group": "gateway.networking.k8s.io", "kind": "Gateway", "name": "public-gateway"}], "hostnames": ["app.example.com"], "rules": [{"matches": [{"path": {"type": "PathPrefix", "value": "/"}}], "backendRefs": [{"name": "frontend-svc", "port": 8080}]}]}}
      }
    },
    "api.UpsertAuthPolicyRequest": {
      "type": "object",
      "required": ["namespace", "name", "spec"],
      "properties": {
        "namespace": {"type": "string", "example": "team-a"},
        "name": {"type": "string", "example": "frontend-authz"},
        "spec": {"type": "object", "example": {"targetRef": {"group": "gateway.networking.k8s.io", "kind": "HTTPRoute", "name": "frontend-route"}, "rules": {"authorization": {"allow": [{"when": [{"key": "request.headers[x-api-key]", "operator": "eq", "values": ["demo-key"]}]}]}}}}
      }
    },
    "api.UpsertRateLimitPolicyRequest": {
      "type": "object",
      "required": ["namespace", "name", "spec"],
      "properties": {
        "namespace": {"type": "string", "example": "team-a"},
        "name": {"type": "string", "example": "frontend-ratelimit"},
        "spec": {"type": "object", "example": {"targetRef": {"group": "gateway.networking.k8s.io", "kind": "HTTPRoute", "name": "frontend-route"}, "limits": {"tenant-default": {"rates": [{"limit": 100, "window": "1m"}]}}}}
      }
    },
    "api.ErrorResponse": {
      "type": "object",
      "properties": {
        "error": {"type": "string"},
        "fieldErrors": {
          "type": "array",
          "items": {"$ref": "#/definitions/validation.FieldError"}
        }
      }
    },
    "validation.FieldError": {
      "type": "object",
      "properties": {
        "field": {"type": "string"},
        "message": {"type": "string"}
      }
    }
  }
}`,
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
