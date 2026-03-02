package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egevorky/qdash/internal/kube"
	"github.com/egevorky/qdash/internal/middleware"
	"github.com/egevorky/qdash/internal/models"
	"github.com/egevorky/qdash/internal/rbac"
	"github.com/egevorky/qdash/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
)

func setupAPIHandlerTest(t *testing.T) (*gin.Engine, *gorm.DB, *rbac.Service, func(models.User)) {
	r, db, rbacSvc, setUser, _, _ := setupAPIHandlerTestWithClients(t)
	return r, db, rbacSvc, setUser
}

func setupAPIHandlerTestWithClients(t *testing.T) (*gin.Engine, *gorm.DB, *rbac.Service, func(models.User), *dynfake.FakeDynamicClient, *k8sfake.Clientset) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.Membership{},
		&models.Group{},
		&models.GroupMember{},
		&models.GroupPermission{},
		&models.Permission{},
		&models.OIDCConfig{},
		&models.OIDCMapping{},
		&models.OrgNamespace{},
		&models.AuditEvent{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	r := gin.New()
	var currentUser *models.User
	r.Use(func(c *gin.Context) {
		if currentUser != nil {
			c.Set(middleware.UserContextKey, *currentUser)
		}
		c.Next()
	})
	rbacSvc := rbac.NewService(db)
	coreClient := k8sfake.NewSimpleClientset()
	dynamicClient := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}: "GatewayClassList",
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}:       "GatewayList",
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}:     "HTTPRouteList",
		{Group: "kuadrant.io", Version: "v1", Resource: "authpolicies"}:                 "AuthPolicyList",
		{Group: "kuadrant.io", Version: "v1", Resource: "ratelimitpolicies"}:            "RateLimitPolicyList",
	})
	kubeClient := &kube.Client{
		Core:    coreClient,
		Dynamic: dynamicClient,
	}
	kubeSvc := service.NewResourceService(kube.NewResourceService(kubeClient))
	h := NewHandler(db, rbacSvc, kubeSvc)
	apiV1 := r.Group("/api/v1")
	h.Register(apiV1)
	return r, db, rbacSvc, func(user models.User) {
		currentUser = &user
	}, dynamicClient, coreClient
}

func TestClusterFailuresReturnBadGateway(t *testing.T) {
	testCases := []struct {
		name      string
		method    string
		path      func(org models.Organization) string
		body      string
		namespace string
		prepare   func(dynamicClient *dynfake.FakeDynamicClient, coreClient *k8sfake.Clientset)
	}{
		{
			name:   "gateway classes list error",
			method: http.MethodGet,
			path: func(org models.Organization) string {
				return "/api/v1/orgs/" + org.Slug + "/gatewayclasses"
			},
			prepare: func(dynamicClient *dynfake.FakeDynamicClient, coreClient *k8sfake.Clientset) {
				dynamicClient.PrependReactor("list", "gatewayclasses", func(action clientgotesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("dynamic list failed")
				})
			},
		},
		{
			name:      "gateway list error",
			method:    http.MethodGet,
			namespace: "team-bg",
			path: func(org models.Organization) string {
				return "/api/v1/orgs/" + org.Slug + "/gateways?namespace=team-bg"
			},
			prepare: func(dynamicClient *dynfake.FakeDynamicClient, coreClient *k8sfake.Clientset) {
				dynamicClient.PrependReactor("list", "gateways", func(action clientgotesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("dynamic list failed")
				})
			},
		},
		{
			name:      "gateway upsert error",
			method:    http.MethodPost,
			namespace: "team-bg",
			path: func(org models.Organization) string {
				return "/api/v1/orgs/" + org.Slug + "/gateways"
			},
			body: `{"namespace":"team-bg","name":"gw-1","spec":{"gatewayClassName":"test-class","listeners":[{"name":"http","port":80,"protocol":"HTTP"}]}}`,
			prepare: func(dynamicClient *dynfake.FakeDynamicClient, coreClient *k8sfake.Clientset) {
				dynamicClient.PrependReactor("get", "gateways", func(action clientgotesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("dynamic get failed")
				})
			},
		},
		{
			name:      "gateway delete error",
			method:    http.MethodDelete,
			namespace: "team-bg",
			path: func(org models.Organization) string {
				return "/api/v1/orgs/" + org.Slug + "/gateways/team-bg/gw-1"
			},
			prepare: func(dynamicClient *dynfake.FakeDynamicClient, coreClient *k8sfake.Clientset) {
				dynamicClient.PrependReactor("delete", "gateways", func(action clientgotesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("dynamic delete failed")
				})
			},
		},
		{
			name:      "namespace create error",
			method:    http.MethodPost,
			namespace: "team-bg",
			path: func(org models.Organization) string {
				return "/api/v1/orgs/" + org.Slug + "/namespaces"
			},
			body: `{"name":"team-bg","profile":"default"}`,
			prepare: func(dynamicClient *dynfake.FakeDynamicClient, coreClient *k8sfake.Clientset) {
				coreClient.PrependReactor("create", "namespaces", func(action clientgotesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("core create failed")
				})
			},
		},
		{
			name:   "namespace adopt lookup error",
			method: http.MethodPost,
			path: func(org models.Organization) string {
				return "/api/v1/orgs/" + org.Slug + "/namespaces/adopt"
			},
			body: `{"name":"team-bg"}`,
			prepare: func(dynamicClient *dynfake.FakeDynamicClient, coreClient *k8sfake.Clientset) {
				coreClient.PrependReactor("get", "namespaces", func(action clientgotesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("core get failed")
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, db, rbacSvc, setUser, dynamicClient, coreClient := setupAPIHandlerTestWithClients(t)

			admin := models.User{Email: "admin-bg-" + strings.ReplaceAll(tc.name, " ", "-") + "@test.local", DisplayName: "admin", Source: "local"}
			if err := db.Create(&admin).Error; err != nil {
				t.Fatalf("create admin: %v", err)
			}
			org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org "+tc.name)
			if err != nil {
				t.Fatalf("create org: %v", err)
			}
			if tc.namespace != "" {
				if err := rbacSvc.ClaimNamespace(org.ID, admin.ID, tc.namespace); err != nil {
					t.Fatalf("claim namespace: %v", err)
				}
			}

			setUser(admin)
			tc.prepare(dynamicClient, coreClient)

			var bodyReader *bytes.Reader
			if strings.TrimSpace(tc.body) == "" {
				bodyReader = bytes.NewReader(nil)
			} else {
				bodyReader = bytes.NewReader([]byte(tc.body))
			}
			req := httptest.NewRequest(tc.method, tc.path(org), bodyReader)
			if strings.TrimSpace(tc.body) != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			res := httptest.NewRecorder()
			r.ServeHTTP(res, req)

			if res.Code != http.StatusBadGateway {
				t.Fatalf("expected 502, got %d: %s", res.Code, res.Body.String())
			}
		})
	}
}

func TestAuditEventsSupportsFiltering(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-audit-filter@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Audit Filter")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	_ = rbacSvc.RecordAuditEvent(org.ID, admin.ID, "resource.apply", "gateways", "success", "ok", map[string]any{"name": "gw-1"})
	_ = rbacSvc.RecordAuditEvent(org.ID, admin.ID, "resource.apply", "gateways", "failed", "failed", map[string]any{"name": "gw-2"})
	_ = rbacSvc.RecordAuditEvent(org.ID, admin.ID, "resource.delete", "httproutes", "success", "ok", map[string]any{"name": "hr-1"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/audit-events?resource=gateways&status=success&eventType=resource.apply", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 listing filtered audit events, got %d: %s", res.Code, res.Body.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly one filtered event, got %d payload=%s", len(out), res.Body.String())
	}
	if out[0]["resource"] != "gateways" || out[0]["status"] != "success" || out[0]["eventType"] != "resource.apply" {
		t.Fatalf("unexpected filtered event payload: %v", out[0])
	}
}

func TestCreateNamespaceUsesOrganizationDefaults(t *testing.T) {
	r, db, rbacSvc, setUser, _, coreClient := setupAPIHandlerTestWithClients(t)

	admin := models.User{Email: "admin-default-ns@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Default Namespace")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	org.Settings = []byte(`{"defaultNamespaceInstance":"canary","defaultNamespaceProfile":"strict-mtls"}`)
	if err := db.Save(&org).Error; err != nil {
		t.Fatalf("save org settings: %v", err)
	}
	setUser(admin)

	body := bytes.NewBufferString(`{"name":"team-defaulted"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/namespaces", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating namespace, got %d: %s", res.Code, res.Body.String())
	}

	ns, err := coreClient.CoreV1().Namespaces().Get(context.Background(), "team-defaulted", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if ns.Labels["istio.io/rev"] != "canary" {
		t.Fatalf("expected istio.io/rev=canary, got %q", ns.Labels["istio.io/rev"])
	}
	if ns.Labels["istio-injection"] != "enabled" {
		t.Fatalf("expected istio-injection=enabled, got %q", ns.Labels["istio-injection"])
	}
	if ns.Labels["security.istio.io/tlsMode"] != "istio" {
		t.Fatalf("expected security.istio.io/tlsMode=istio, got %q", ns.Labels["security.istio.io/tlsMode"])
	}
}

func TestCreateNamespaceRequestOverridesOrganizationDefaults(t *testing.T) {
	r, db, rbacSvc, setUser, _, coreClient := setupAPIHandlerTestWithClients(t)

	admin := models.User{Email: "admin-override-ns@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Override Namespace")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	org.Settings = []byte(`{"defaultNamespaceInstance":"canary","defaultNamespaceProfile":"strict-mtls"}`)
	if err := db.Save(&org).Error; err != nil {
		t.Fatalf("save org settings: %v", err)
	}
	setUser(admin)

	body := bytes.NewBufferString(`{"name":"team-override","instance":"default","profile":"ambient"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/namespaces", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating namespace, got %d: %s", res.Code, res.Body.String())
	}

	ns, err := coreClient.CoreV1().Namespaces().Get(context.Background(), "team-override", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if ns.Labels["istio.io/rev"] != "default" {
		t.Fatalf("expected istio.io/rev=default from request override, got %q", ns.Labels["istio.io/rev"])
	}
	if ns.Labels["istio.io/dataplane-mode"] != "ambient" {
		t.Fatalf("expected istio.io/dataplane-mode=ambient from request override, got %q", ns.Labels["istio.io/dataplane-mode"])
	}
	if _, ok := ns.Labels["security.istio.io/tlsMode"]; ok {
		t.Fatalf("did not expect strict-mtls label after profile override: %+v", ns.Labels)
	}
}

func TestAdoptNamespaceRequiresAdmin(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Admin")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	editorEmail := "editor@test.local"
	if err := rbacSvc.UpsertMembership(org.ID, editorEmail, "editor", nil); err != nil {
		t.Fatalf("create editor membership: %v", err)
	}
	var editor models.User
	if err := db.Where("email = ?", editorEmail).First(&editor).Error; err != nil {
		t.Fatalf("load editor: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"name": "existing-ns"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/namespaces/adopt", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	setUser(editor)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin adoption, got %d: %s", res.Code, res.Body.String())
	}
}

func TestResourceUpsertDeniedForUnownedNamespace(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	adminA := models.User{Email: "a@test.local", DisplayName: "a", Source: "local"}
	adminB := models.User{Email: "b@test.local", DisplayName: "b", Source: "local"}
	if err := db.Create(&adminA).Error; err != nil {
		t.Fatalf("create adminA: %v", err)
	}
	if err := db.Create(&adminB).Error; err != nil {
		t.Fatalf("create adminB: %v", err)
	}

	orgA, err := rbacSvc.CreateOrganizationWithAdmin(adminA, "Org A")
	if err != nil {
		t.Fatalf("create orgA: %v", err)
	}
	orgB, err := rbacSvc.CreateOrganizationWithAdmin(adminB, "Org B")
	if err != nil {
		t.Fatalf("create orgB: %v", err)
	}

	if err := rbacSvc.ClaimNamespace(orgA.ID, adminA.ID, "team-a"); err != nil {
		t.Fatalf("claim namespace orgA: %v", err)
	}

	payload := map[string]any{
		"namespace": "team-a",
		"name":      "gw-b",
		"spec": map[string]any{
			"gatewayClassName": "test-class",
			"listeners":        []any{map[string]any{"name": "http", "port": 80, "protocol": "HTTP"}},
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+orgB.Slug+"/gateways", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	setUser(adminB)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unowned namespace upsert, got %d: %s", res.Code, res.Body.String())
	}
}

func TestPermissionEndpointsAdminViewerMatrix(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-matrix@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Matrix")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := rbacSvc.UpsertMembership(org.ID, "viewer-matrix@test.local", "viewer", nil); err != nil {
		t.Fatalf("create viewer membership: %v", err)
	}
	var viewer models.User
	if err := db.Where("email = ?", "viewer-matrix@test.local").First(&viewer).Error; err != nil {
		t.Fatalf("load viewer: %v", err)
	}

	// viewer can read permissions
	setUser(viewer)
	readReq := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/permissions", nil)
	readRes := httptest.NewRecorder()
	r.ServeHTTP(readRes, readReq)
	if readRes.Code != http.StatusOK {
		t.Fatalf("viewer expected 200 on GET permissions, got %d: %s", readRes.Code, readRes.Body.String())
	}

	// viewer cannot create permissions
	createBody, _ := json.Marshal(map[string]any{"name": "custom.read", "resource": "custom", "action": "read", "definition": "viewer should not create"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/permissions", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	r.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusForbidden {
		t.Fatalf("viewer expected 403 on POST permissions, got %d: %s", createRes.Code, createRes.Body.String())
	}

	// admin can create permissions
	setUser(admin)
	adminCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/permissions", bytes.NewReader(createBody))
	adminCreateReq.Header.Set("Content-Type", "application/json")
	adminCreateRes := httptest.NewRecorder()
	r.ServeHTTP(adminCreateRes, adminCreateReq)
	if adminCreateRes.Code != http.StatusCreated {
		t.Fatalf("admin expected 201 on POST permissions, got %d: %s", adminCreateRes.Code, adminCreateRes.Body.String())
	}
}

func TestGatewayUpsertEditorAllowedViewerDenied(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-gw@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org GW")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := rbacSvc.ClaimNamespace(org.ID, admin.ID, "team-gw"); err != nil {
		t.Fatalf("claim namespace: %v", err)
	}
	if err := rbacSvc.UpsertMembership(org.ID, "editor-gw@test.local", "editor", nil); err != nil {
		t.Fatalf("create editor membership: %v", err)
	}
	if err := rbacSvc.UpsertMembership(org.ID, "viewer-gw@test.local", "viewer", nil); err != nil {
		t.Fatalf("create viewer membership: %v", err)
	}
	var editor models.User
	if err := db.Where("email = ?", "editor-gw@test.local").First(&editor).Error; err != nil {
		t.Fatalf("load editor: %v", err)
	}
	var viewer models.User
	if err := db.Where("email = ?", "viewer-gw@test.local").First(&viewer).Error; err != nil {
		t.Fatalf("load viewer: %v", err)
	}

	payload := map[string]any{
		"namespace": "team-gw",
		"name":      "gw-1",
		"spec": map[string]any{
			"gatewayClassName": "test-class",
			"listeners":        []any{map[string]any{"name": "http", "port": 80, "protocol": "HTTP"}},
		},
	}
	body, _ := json.Marshal(payload)

	// editor can upsert
	setUser(editor)
	editorReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/gateways", bytes.NewReader(body))
	editorReq.Header.Set("Content-Type", "application/json")
	editorRes := httptest.NewRecorder()
	r.ServeHTTP(editorRes, editorReq)
	if editorRes.Code != http.StatusOK {
		t.Fatalf("editor expected 200 on gateway upsert, got %d: %s", editorRes.Code, editorRes.Body.String())
	}

	// viewer cannot upsert
	setUser(viewer)
	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/gateways", bytes.NewReader(body))
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerRes := httptest.NewRecorder()
	r.ServeHTTP(viewerRes, viewerReq)
	if viewerRes.Code != http.StatusForbidden {
		t.Fatalf("viewer expected 403 on gateway upsert, got %d: %s", viewerRes.Code, viewerRes.Body.String())
	}
}

func TestGatewayUpsertAllowedViaGroupPermission(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-group@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Group")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := rbacSvc.ClaimNamespace(org.ID, admin.ID, "team-group"); err != nil {
		t.Fatalf("claim namespace: %v", err)
	}
	if err := rbacSvc.UpsertMembership(org.ID, "viewer-group@test.local", "viewer", nil); err != nil {
		t.Fatalf("create viewer membership: %v", err)
	}
	var viewer models.User
	if err := db.Where("email = ?", "viewer-group@test.local").First(&viewer).Error; err != nil {
		t.Fatalf("load viewer: %v", err)
	}

	group, err := rbacSvc.CreateGroup(org.ID, "gateway-editors")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := rbacSvc.AddGroupMemberByEmail(org.ID, group.ID, viewer.Email); err != nil {
		t.Fatalf("add group member: %v", err)
	}
	if err := rbacSvc.AddGroupPermission(org.ID, group.ID, "gateway.write"); err != nil {
		t.Fatalf("add group permission: %v", err)
	}

	payload := map[string]any{
		"namespace": "team-group",
		"name":      "gw-group",
		"spec": map[string]any{
			"gatewayClassName": "test-class",
			"listeners":        []any{map[string]any{"name": "http", "port": 80, "protocol": "HTTP"}},
		},
	}
	body, _ := json.Marshal(payload)

	setUser(viewer)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/gateways", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("viewer with group gateway.write expected 200, got %d: %s", res.Code, res.Body.String())
	}
}

func TestGatewayUpsertReturnsFieldErrorsOnSemanticValidation(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-semantic@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Semantic")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := rbacSvc.ClaimNamespace(org.ID, admin.ID, "team-semantic"); err != nil {
		t.Fatalf("claim namespace: %v", err)
	}
	setUser(admin)

	body, _ := json.Marshal(map[string]any{
		"namespace": "team-semantic",
		"name":      "gw-invalid",
		"spec": map[string]any{
			"listeners": []any{
				map[string]any{"name": "http", "protocol": "HTTP", "port": 80},
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/gateways", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for semantic validation, got %d: %s", res.Code, res.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if _, ok := out["fieldErrors"]; !ok {
		t.Fatalf("expected fieldErrors in response body, got %v", out)
	}
}

func TestGroupEndpointsCRUD(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-groups-crud@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Groups CRUD")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	createBody := bytes.NewBufferString(`{"name":"ops-team"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/rbac/groups", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	r.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating group, got %d: %s", createRes.Code, createRes.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/rbac/groups", nil)
	listRes := httptest.NewRecorder()
	r.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected 200 listing groups, got %d: %s", listRes.Code, listRes.Body.String())
	}
	if !bytes.Contains(listRes.Body.Bytes(), []byte("ops-team")) {
		t.Fatalf("expected group name in list response: %s", listRes.Body.String())
	}
}

func TestGroupEndpointLifecycleJSON(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-group-lifecycle@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Group Lifecycle")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	// create group
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/rbac/groups", strings.NewReader(`{"name":"sec-team"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	r.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating group, got %d: %s", createRes.Code, createRes.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create group response: %v", err)
	}
	if created["name"] != "sec-team" {
		t.Fatalf("expected group name sec-team, got %v", created["name"])
	}
	groupID, _ := created["id"].(string)
	if strings.TrimSpace(groupID) == "" {
		t.Fatalf("expected non-empty group id in response: %v", created)
	}

	// add group permission
	addPermReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/rbac/groups/"+groupID+"/permissions", strings.NewReader(`{"permission":"security.write"}`))
	addPermReq.Header.Set("Content-Type", "application/json")
	addPermRes := httptest.NewRecorder()
	r.ServeHTTP(addPermRes, addPermReq)
	if addPermRes.Code != http.StatusCreated {
		t.Fatalf("expected 201 add group permission, got %d: %s", addPermRes.Code, addPermRes.Body.String())
	}
	var createdPerm map[string]any
	if err := json.Unmarshal(addPermRes.Body.Bytes(), &createdPerm); err != nil {
		t.Fatalf("decode add permission response: %v", err)
	}
	if createdPerm["permission"] != "security.write" {
		t.Fatalf("expected permission security.write, got %v", createdPerm["permission"])
	}

	// add group member
	addMemberReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/rbac/groups/"+groupID+"/users", strings.NewReader(`{"email":"user-group-lifecycle@test.local"}`))
	addMemberReq.Header.Set("Content-Type", "application/json")
	addMemberRes := httptest.NewRecorder()
	r.ServeHTTP(addMemberRes, addMemberReq)
	if addMemberRes.Code != http.StatusCreated {
		t.Fatalf("expected 201 add group member, got %d: %s", addMemberRes.Code, addMemberRes.Body.String())
	}
	var createdMember map[string]any
	if err := json.Unmarshal(addMemberRes.Body.Bytes(), &createdMember); err != nil {
		t.Fatalf("decode add member response: %v", err)
	}
	if createdMember["email"] != "user-group-lifecycle@test.local" {
		t.Fatalf("expected added member email, got %v", createdMember["email"])
	}
	userID, _ := createdMember["id"].(string)
	if strings.TrimSpace(userID) == "" {
		t.Fatalf("expected non-empty user id in add member response: %v", createdMember)
	}

	// list groups
	listGroupsReq := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/rbac/groups", nil)
	listGroupsRes := httptest.NewRecorder()
	r.ServeHTTP(listGroupsRes, listGroupsReq)
	if listGroupsRes.Code != http.StatusOK {
		t.Fatalf("expected 200 list groups, got %d: %s", listGroupsRes.Code, listGroupsRes.Body.String())
	}
	var groups []map[string]any
	if err := json.Unmarshal(listGroupsRes.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode list groups response: %v", err)
	}
	if len(groups) != 1 || groups[0]["name"] != "sec-team" {
		t.Fatalf("unexpected groups payload: %v", groups)
	}

	// list group permissions
	listPermReq := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/rbac/groups/"+groupID+"/permissions", nil)
	listPermRes := httptest.NewRecorder()
	r.ServeHTTP(listPermRes, listPermReq)
	if listPermRes.Code != http.StatusOK {
		t.Fatalf("expected 200 list group permissions, got %d: %s", listPermRes.Code, listPermRes.Body.String())
	}
	var perms []map[string]any
	if err := json.Unmarshal(listPermRes.Body.Bytes(), &perms); err != nil {
		t.Fatalf("decode list permissions response: %v", err)
	}
	if len(perms) != 1 || perms[0]["permission"] != "security.write" {
		t.Fatalf("unexpected permissions payload: %v", perms)
	}

	// list group members
	listMemberReq := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/rbac/groups/"+groupID+"/users", nil)
	listMemberRes := httptest.NewRecorder()
	r.ServeHTTP(listMemberRes, listMemberReq)
	if listMemberRes.Code != http.StatusOK {
		t.Fatalf("expected 200 list group members, got %d: %s", listMemberRes.Code, listMemberRes.Body.String())
	}
	var members []map[string]any
	if err := json.Unmarshal(listMemberRes.Body.Bytes(), &members); err != nil {
		t.Fatalf("decode list members response: %v", err)
	}
	if len(members) != 1 || members[0]["email"] != "user-group-lifecycle@test.local" {
		t.Fatalf("unexpected members payload: %v", members)
	}

	// remove group permission
	removePermReq := httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/rbac/groups/"+groupID+"/permissions/security.write", nil)
	removePermRes := httptest.NewRecorder()
	r.ServeHTTP(removePermRes, removePermReq)
	if removePermRes.Code != http.StatusOK {
		t.Fatalf("expected 200 remove group permission, got %d: %s", removePermRes.Code, removePermRes.Body.String())
	}
	var removePermStatus map[string]any
	if err := json.Unmarshal(removePermRes.Body.Bytes(), &removePermStatus); err != nil {
		t.Fatalf("decode remove permission response: %v", err)
	}
	if removePermStatus["status"] != "deleted" {
		t.Fatalf("expected deleted status removing permission, got %v", removePermStatus)
	}

	// remove group member
	removeMemberReq := httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/rbac/groups/"+groupID+"/users/"+userID, nil)
	removeMemberRes := httptest.NewRecorder()
	r.ServeHTTP(removeMemberRes, removeMemberReq)
	if removeMemberRes.Code != http.StatusOK {
		t.Fatalf("expected 200 remove group member, got %d: %s", removeMemberRes.Code, removeMemberRes.Body.String())
	}
	var removeMemberStatus map[string]any
	if err := json.Unmarshal(removeMemberRes.Body.Bytes(), &removeMemberStatus); err != nil {
		t.Fatalf("decode remove member response: %v", err)
	}
	if removeMemberStatus["status"] != "deleted" {
		t.Fatalf("expected deleted status removing member, got %v", removeMemberStatus)
	}

	// delete group
	deleteGroupReq := httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/rbac/groups/"+groupID, nil)
	deleteGroupRes := httptest.NewRecorder()
	r.ServeHTTP(deleteGroupRes, deleteGroupReq)
	if deleteGroupRes.Code != http.StatusOK {
		t.Fatalf("expected 200 delete group, got %d: %s", deleteGroupRes.Code, deleteGroupRes.Body.String())
	}
	var deleteGroupStatus map[string]any
	if err := json.Unmarshal(deleteGroupRes.Body.Bytes(), &deleteGroupStatus); err != nil {
		t.Fatalf("decode delete group response: %v", err)
	}
	if deleteGroupStatus["status"] != "deleted" {
		t.Fatalf("expected deleted status deleting group, got %v", deleteGroupStatus)
	}
}

func TestGroupEndpointsInvalidGroupID(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-group-invalid-id@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Group Invalid ID")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/rbac/groups/not-a-uuid/users", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid group id, got %d: %s", res.Code, res.Body.String())
	}
}

func TestGroupEndpointsUnknownPermissionRejected(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-group-unknown-perm@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Group Unknown Perm")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	createGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/rbac/groups", strings.NewReader(`{"name":"ops"}`))
	createGroupReq.Header.Set("Content-Type", "application/json")
	createGroupRes := httptest.NewRecorder()
	r.ServeHTTP(createGroupRes, createGroupReq)
	if createGroupRes.Code != http.StatusCreated {
		t.Fatalf("expected 201 create group, got %d: %s", createGroupRes.Code, createGroupRes.Body.String())
	}
	var group map[string]any
	_ = json.Unmarshal(createGroupRes.Body.Bytes(), &group)
	groupID, _ := group["id"].(string)

	addPermReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/rbac/groups/"+groupID+"/permissions", strings.NewReader(`{"permission":"nonexistent.permission"}`))
	addPermReq.Header.Set("Content-Type", "application/json")
	addPermRes := httptest.NewRecorder()
	r.ServeHTTP(addPermRes, addPermReq)
	if addPermRes.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 unknown permission, got %d: %s", addPermRes.Code, addPermRes.Body.String())
	}
}

func TestGroupEndpointsNonMemberDenied(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-group-non-member@test.local", DisplayName: "admin", Source: "local"}
	outsider := models.User{Email: "outsider-group-non-member@test.local", DisplayName: "outsider", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := db.Create(&outsider).Error; err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Group Non Member")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	setUser(outsider)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/rbac/groups", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member access, got %d: %s", res.Code, res.Body.String())
	}
}

func TestGroupEndpointsCrossOrgIsolation(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	adminA := models.User{Email: "admin-a-group-iso@test.local", DisplayName: "admin-a", Source: "local"}
	adminB := models.User{Email: "admin-b-group-iso@test.local", DisplayName: "admin-b", Source: "local"}
	if err := db.Create(&adminA).Error; err != nil {
		t.Fatalf("create adminA: %v", err)
	}
	if err := db.Create(&adminB).Error; err != nil {
		t.Fatalf("create adminB: %v", err)
	}
	orgA, err := rbacSvc.CreateOrganizationWithAdmin(adminA, "Org A Group Iso")
	if err != nil {
		t.Fatalf("create orgA: %v", err)
	}
	orgB, err := rbacSvc.CreateOrganizationWithAdmin(adminB, "Org B Group Iso")
	if err != nil {
		t.Fatalf("create orgB: %v", err)
	}

	setUser(adminA)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+orgA.Slug+"/rbac/groups", strings.NewReader(`{"name":"org-a-group"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	r.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected 201 create group in orgA, got %d: %s", createRes.Code, createRes.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(createRes.Body.Bytes(), &created)
	groupID, _ := created["id"].(string)

	setUser(adminB)
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+orgB.Slug+"/rbac/groups/"+groupID, nil)
	deleteRes := httptest.NewRecorder()
	r.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusNotFound {
		t.Fatalf("expected 404 deleting cross-org group, got %d: %s", deleteRes.Code, deleteRes.Body.String())
	}
	var deleteErr map[string]any
	if err := json.Unmarshal(deleteRes.Body.Bytes(), &deleteErr); err != nil {
		t.Fatalf("decode cross-org delete error: %v", err)
	}
	if deleteErr["error"] != "group not found" {
		t.Fatalf("expected group not found error, got %v", deleteErr["error"])
	}
}

func TestGroupEndpointsMissingGroupMessageConsistency(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-group-missing-msg@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Group Missing Msg")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	// valid UUID not present in this org
	missingGroupID := "11111111-1111-1111-1111-111111111111"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/rbac/groups/"+missingGroupID+"/users", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 listing missing group users, got %d: %s", res.Code, res.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode missing group error: %v", err)
	}
	if out["error"] != "group not found" {
		t.Fatalf("expected group not found error, got %v", out["error"])
	}
}

func TestGroupEndpointsMissingGroupConsistencyAllOperations(t *testing.T) {
	r, db, rbacSvc, setUser := setupAPIHandlerTest(t)

	admin := models.User{Email: "admin-group-missing-all@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Group Missing All")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	missingGroupID := "11111111-1111-1111-1111-111111111111"
	validUserID := "22222222-2222-2222-2222-222222222222"

	cases := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
	}{
		{
			name:   "list group users",
			method: http.MethodGet,
			path:   "/api/v1/orgs/" + org.Slug + "/rbac/groups/" + missingGroupID + "/users",
		},
		{
			name:        "add group user",
			method:      http.MethodPost,
			path:        "/api/v1/orgs/" + org.Slug + "/rbac/groups/" + missingGroupID + "/users",
			body:        `{"email":"missing-group-user@test.local"}`,
			contentType: "application/json",
		},
		{
			name:   "remove group user",
			method: http.MethodDelete,
			path:   "/api/v1/orgs/" + org.Slug + "/rbac/groups/" + missingGroupID + "/users/" + validUserID,
		},
		{
			name:   "list group permissions",
			method: http.MethodGet,
			path:   "/api/v1/orgs/" + org.Slug + "/rbac/groups/" + missingGroupID + "/permissions",
		},
		{
			name:        "add group permission",
			method:      http.MethodPost,
			path:        "/api/v1/orgs/" + org.Slug + "/rbac/groups/" + missingGroupID + "/permissions",
			body:        `{"permission":"gateway.write"}`,
			contentType: "application/json",
		},
		{
			name:   "remove group permission",
			method: http.MethodDelete,
			path:   "/api/v1/orgs/" + org.Slug + "/rbac/groups/" + missingGroupID + "/permissions/gateway.write",
		},
		{
			name:   "delete group",
			method: http.MethodDelete,
			path:   "/api/v1/orgs/" + org.Slug + "/rbac/groups/" + missingGroupID,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			res := httptest.NewRecorder()
			r.ServeHTTP(res, req)

			if res.Code != http.StatusNotFound {
				t.Fatalf("expected 404 missing group, got %d: %s", res.Code, res.Body.String())
			}
			var out map[string]any
			if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode missing group error: %v", err)
			}
			if out["error"] != "group not found" {
				t.Fatalf("expected group not found, got %v", out["error"])
			}
		})
	}
}
