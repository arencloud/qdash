package web

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/arencloud/qdash/internal/kube"
	"github.com/arencloud/qdash/internal/middleware"
	"github.com/arencloud/qdash/internal/models"
	"github.com/arencloud/qdash/internal/rbac"
	"github.com/arencloud/qdash/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupWebHandlerTest(t *testing.T) (*gin.Engine, *rbac.Service, *gorm.DB, *k8sfake.Clientset, func(models.User)) {
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
		&models.AuthSession{},
		&models.OIDCAuthRequest{},
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

	tmpl := template.Must(template.New("test").Parse(`
{{define "flash"}}<div class="flash">{{.Message}}</div>{{end}}
{{define "validation_errors"}}<div>Validation errors:{{range .Errors}}<span>{{.Field}}:{{.Message}}</span>{{end}}</div>{{end}}
{{define "namespace_panel"}}<div id="ns-panel">{{range .Namespaces}}<span class="ns">{{.Name}}</span>{{else}}<span>none</span>{{end}}</div>{{end}}
{{define "org_rbac"}}<div id="rbac-page">{{.Slug}}</div>{{end}}
{{define "rbac_panel"}}<div id="rbac-panel">
{{range .Memberships}}<span class="member">{{.Email}}|{{.Role}}|{{.Permission}}</span>{{end}}
{{range .Groups}}
<div class="group">{{.Name}}</div>
{{range .Members}}<span class="group-member">{{.Email}}</span>{{end}}
{{range .Permissions}}<span class="group-perm">{{.}}</span>{{end}}
{{end}}
</div>{{end}}
`))
	r.SetHTMLTemplate(tmpl)

	rbacSvc := rbac.NewService(db)
	coreClient := k8sfake.NewSimpleClientset()
	kubeClient := &kube.Client{Core: coreClient}
	kubeSvc := service.NewResourceService(kube.NewResourceService(kubeClient))

	h := NewHandler(rbacSvc, service.NewAuthService(db), nil, kubeSvc)
	h.RegisterProtected(r.Group("/"))

	return r, rbacSvc, db, coreClient, func(user models.User) {
		currentUser = &user
	}
}

func TestResourceNamespaceCreateClaimsOwnership(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-create@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Create")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	form := url.Values{}
	form.Set("name", "team-create")
	form.Set("discovery_label", "default")
	form.Set("revision_tag", "default")
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/resources/namespaces/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "namespace created and linked") {
		t.Fatalf("unexpected response body: %s", res.Body.String())
	}
	belongs, err := rbacSvc.NamespaceBelongsToOrg(org.ID, "team-create")
	if err != nil || !belongs {
		t.Fatalf("namespace was not claimed for org")
	}
}

func TestResourceNamespaceCreateUsesOrganizationDefaults(t *testing.T) {
	r, rbacSvc, db, coreClient, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-create-defaults@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Create Defaults")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	org.Settings = []byte(`{"defaultIstioDiscoveryLabel":"default","defaultIstioRevisionTag":"canary"}`)
	if err := db.Save(&org).Error; err != nil {
		t.Fatalf("save org settings: %v", err)
	}
	setUser(admin)

	form := url.Values{}
	form.Set("name", "team-create-defaults")
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/resources/namespaces/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}

	ns, err := coreClient.CoreV1().Namespaces().Get(req.Context(), "team-create-defaults", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("load created namespace: %v", err)
	}
	if ns.Labels["istio.io/rev"] != "canary" {
		t.Fatalf("expected istio.io/rev=canary, got %q", ns.Labels["istio.io/rev"])
	}
	if ns.Labels["istio-discovery"] != "default" {
		t.Fatalf("expected istio-discovery=default, got %q", ns.Labels["istio-discovery"])
	}
}

func TestResourceNamespaceCreateRequestOverridesOrganizationDefaults(t *testing.T) {
	r, rbacSvc, db, coreClient, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-create-override@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Create Override")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	org.Settings = []byte(`{"defaultIstioDiscoveryLabel":"default","defaultIstioRevisionTag":"canary"}`)
	if err := db.Save(&org).Error; err != nil {
		t.Fatalf("save org settings: %v", err)
	}
	setUser(admin)

	form := url.Values{}
	form.Set("name", "team-create-override")
	form.Set("discovery_label", "mesh-a")
	form.Set("revision_tag", "prod-stable")
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/resources/namespaces/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}

	ns, err := coreClient.CoreV1().Namespaces().Get(req.Context(), "team-create-override", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("load created namespace: %v", err)
	}
	if ns.Labels["istio.io/rev"] != "prod-stable" {
		t.Fatalf("expected istio.io/rev=prod-stable, got %q", ns.Labels["istio.io/rev"])
	}
	if ns.Labels["istio-discovery"] != "mesh-a" {
		t.Fatalf("expected istio-discovery=mesh-a, got %q", ns.Labels["istio-discovery"])
	}
	if ns.Labels["istio.io/rev"] == "canary" {
		t.Fatalf("did not expect canary revision after explicit override: %+v", ns.Labels)
	}
}

func TestResourceNamespaceAdoptRequiresAdmin(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-adopt@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Adopt")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := rbacSvc.UpsertMembership(org.ID, "editor-adopt@test.local", "editor", nil); err != nil {
		t.Fatalf("create editor membership: %v", err)
	}
	var editor models.User
	if err := db.Where("email = ?", "editor-adopt@test.local").First(&editor).Error; err != nil {
		t.Fatalf("load editor: %v", err)
	}
	setUser(editor)

	form := url.Values{}
	form.Set("name", "existing-ns")
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/resources/namespaces/adopt", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin adopt, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "admin role required") {
		t.Fatalf("expected admin role message, got: %s", res.Body.String())
	}
}

func TestResourceApplyRendersFieldValidationErrors(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-validate@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Validate")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := rbacSvc.ClaimNamespace(org.ID, admin.ID, "team-validate"); err != nil {
		t.Fatalf("claim namespace: %v", err)
	}
	setUser(admin)

	form := url.Values{}
	form.Set("namespace", "team-validate")
	form.Set("name", "gw-test")
	form.Set("listener_name", "http")
	form.Set("protocol", "HTTP")
	form.Set("port", "80")
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/resources/gateways/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for validation errors, got %d: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, "Validation errors") || !strings.Contains(body, "gateway_class") {
		t.Fatalf("expected field validation output, got: %s", body)
	}
}

func TestResourceApplyRendersSemanticValidationErrors(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-semantic-web@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Semantic Web")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := rbacSvc.ClaimNamespace(org.ID, admin.ID, "team-semantic-web"); err != nil {
		t.Fatalf("claim namespace: %v", err)
	}
	setUser(admin)

	form := url.Values{}
	form.Set("namespace", "team-semantic-web")
	form.Set("name", "gw-semantic")
	form.Set("spec_json", `{"listeners":[{"name":"http","protocol":"HTTP","port":80}]}`)
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/resources/gateways/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for semantic validation errors, got %d: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, "Validation errors") || !strings.Contains(body, "spec.gatewayClassName") {
		t.Fatalf("expected semantic field validation output, got: %s", body)
	}
}

func TestNamespacePanelRefreshAfterCreate(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-panel-create@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Panel Create")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	form := url.Values{}
	form.Set("name", "team-refresh")
	form.Set("discovery_label", "default")
	form.Set("revision_tag", "default")
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/resources/namespaces/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Header().Get("HX-Trigger"), "namespaceChanged") {
		t.Fatalf("expected HX-Trigger to include namespaceChanged, got %q", res.Header().Get("HX-Trigger"))
	}

	panelReq := httptest.NewRequest(http.MethodGet, "/organizations/"+org.Slug+"/resources/namespaces/panel", nil)
	panelRes := httptest.NewRecorder()
	r.ServeHTTP(panelRes, panelReq)
	if panelRes.Code != http.StatusOK {
		t.Fatalf("expected panel 200, got %d: %s", panelRes.Code, panelRes.Body.String())
	}
	if !strings.Contains(panelRes.Body.String(), "team-refresh") {
		t.Fatalf("expected namespace panel to include team-refresh, got: %s", panelRes.Body.String())
	}
}

func TestNamespacePanelRefreshAfterAdopt(t *testing.T) {
	r, rbacSvc, db, coreClient, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-panel-adopt@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org Panel Adopt")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	if _, err := coreClient.CoreV1().Namespaces().Create(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "existing-refresh"}},
		metav1.CreateOptions{},
	); err != nil {
		t.Fatalf("seed cluster namespace: %v", err)
	}

	form := url.Values{}
	form.Set("name", "existing-refresh")
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/resources/namespaces/adopt", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected adopt 200, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Header().Get("HX-Trigger"), "namespaceChanged") {
		t.Fatalf("expected HX-Trigger to include namespaceChanged, got %q", res.Header().Get("HX-Trigger"))
	}

	panelReq := httptest.NewRequest(http.MethodGet, "/organizations/"+org.Slug+"/resources/namespaces/panel", nil)
	panelRes := httptest.NewRecorder()
	r.ServeHTTP(panelRes, panelReq)
	if panelRes.Code != http.StatusOK {
		t.Fatalf("expected panel 200, got %d: %s", panelRes.Code, panelRes.Body.String())
	}
	if !strings.Contains(panelRes.Body.String(), "existing-refresh") {
		t.Fatalf("expected namespace panel to include existing-refresh, got: %s", panelRes.Body.String())
	}
}

func TestRBACGroupHTMXFlow(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-rbac-group@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org RBAC Group")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	createGroupForm := url.Values{}
	createGroupForm.Set("name", "gateway-editors")
	createGroupReq := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/rbac/groups", strings.NewReader(createGroupForm.Encode()))
	createGroupReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createGroupRes := httptest.NewRecorder()
	r.ServeHTTP(createGroupRes, createGroupReq)
	if createGroupRes.Code != http.StatusOK {
		t.Fatalf("expected 200 creating group, got %d: %s", createGroupRes.Code, createGroupRes.Body.String())
	}
	if !strings.Contains(createGroupRes.Header().Get("HX-Trigger"), "rbacChanged") {
		t.Fatalf("expected HX-Trigger rbacChanged, got %q", createGroupRes.Header().Get("HX-Trigger"))
	}

	groups, err := rbacSvc.ListGroups(org.ID)
	if err != nil || len(groups) != 1 {
		t.Fatalf("expected one group after create, err=%v len=%d", err, len(groups))
	}
	groupID := groups[0].ID.String()

	addPermForm := url.Values{}
	addPermForm.Set("permission", "gateway.write")
	addPermReq := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/rbac/groups/"+groupID+"/permissions", strings.NewReader(addPermForm.Encode()))
	addPermReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addPermRes := httptest.NewRecorder()
	r.ServeHTTP(addPermRes, addPermReq)
	if addPermRes.Code != http.StatusOK {
		t.Fatalf("expected 200 adding group permission, got %d: %s", addPermRes.Code, addPermRes.Body.String())
	}

	addMemberForm := url.Values{}
	addMemberForm.Set("email", "viewer-rbac-group@test.local")
	addMemberReq := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/rbac/groups/"+groupID+"/users", strings.NewReader(addMemberForm.Encode()))
	addMemberReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addMemberRes := httptest.NewRecorder()
	r.ServeHTTP(addMemberRes, addMemberReq)
	if addMemberRes.Code != http.StatusOK {
		t.Fatalf("expected 200 adding group member, got %d: %s", addMemberRes.Code, addMemberRes.Body.String())
	}

	members, err := rbacSvc.ListGroupMembers(org.ID, groups[0].ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("expected one group member, err=%v len=%d", err, len(members))
	}
	removeMemberReq := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/rbac/groups/"+groupID+"/users/"+members[0].ID.String()+"/delete", strings.NewReader(""))
	removeMemberReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	removeMemberRes := httptest.NewRecorder()
	r.ServeHTTP(removeMemberRes, removeMemberReq)
	if removeMemberRes.Code != http.StatusOK {
		t.Fatalf("expected 200 removing group member, got %d: %s", removeMemberRes.Code, removeMemberRes.Body.String())
	}

	deleteGroupReq := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/rbac/groups/"+groupID+"/delete", strings.NewReader(""))
	deleteGroupReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	deleteGroupRes := httptest.NewRecorder()
	r.ServeHTTP(deleteGroupRes, deleteGroupReq)
	if deleteGroupRes.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting group, got %d: %s", deleteGroupRes.Code, deleteGroupRes.Body.String())
	}
}

func TestRBACPanelRendersGroupData(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-rbac-panel@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org RBAC Panel")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	if err := rbacSvc.UpsertMembership(org.ID, "viewer-panel@test.local", "viewer", []string{"security.read"}); err != nil {
		t.Fatalf("create membership: %v", err)
	}
	group, err := rbacSvc.CreateGroup(org.ID, "gateway-editors")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := rbacSvc.AddGroupMemberByEmail(org.ID, group.ID, "viewer-panel@test.local"); err != nil {
		t.Fatalf("add group member: %v", err)
	}
	if err := rbacSvc.AddGroupPermission(org.ID, group.ID, "gateway.write"); err != nil {
		t.Fatalf("add group permission: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/organizations/"+org.Slug+"/rbac/panel", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 rendering rbac panel, got %d: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, "viewer-panel@test.local") {
		t.Fatalf("expected member email in panel body, got: %s", body)
	}
	if !strings.Contains(body, "gateway-editors") {
		t.Fatalf("expected group name in panel body, got: %s", body)
	}
	if !strings.Contains(body, "gateway.write") {
		t.Fatalf("expected group permission in panel body, got: %s", body)
	}
}

func TestRBACPanelForbiddenForNonMember(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-rbac-panel-forbid@test.local", DisplayName: "admin", Source: "local"}
	outsider := models.User{Email: "outsider-rbac-panel@test.local", DisplayName: "outsider", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := db.Create(&outsider).Error; err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org RBAC Forbidden")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	setUser(outsider)
	req := httptest.NewRequest(http.MethodGet, "/organizations/"+org.Slug+"/rbac/panel", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member rbac panel, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "forbidden") {
		t.Fatalf("expected forbidden message, got: %s", res.Body.String())
	}
}

func TestRBACGroupInvalidIDReturnsBadRequest(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-rbac-invalid-groupid@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org RBAC Invalid ID")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	form := url.Values{}
	form.Set("permission", "gateway.write")
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/rbac/groups/not-a-uuid/permissions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid group id, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "invalid group id") {
		t.Fatalf("expected invalid group id message, got: %s", res.Body.String())
	}
}

func TestRBACGroupCreateForbiddenForNonMember(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-rbac-forbidden@test.local", DisplayName: "admin", Source: "local"}
	outsider := models.User{Email: "outsider-rbac-forbidden@test.local", DisplayName: "outsider", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := db.Create(&outsider).Error; err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org RBAC Forbidden Group")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(outsider)

	form := url.Values{}
	form.Set("name", "ops-team")
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/rbac/groups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member group create, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "forbidden") {
		t.Fatalf("expected forbidden message, got: %s", res.Body.String())
	}
}

func TestRBACGroupUnknownPermissionRejected(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	admin := models.User{Email: "admin-rbac-unknown-perm@test.local", DisplayName: "admin", Source: "local"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := rbacSvc.CreateOrganizationWithAdmin(admin, "Org RBAC Unknown Perm")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	setUser(admin)

	group, err := rbacSvc.CreateGroup(org.ID, "ops-team")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	form := url.Values{}
	form.Set("permission", "nonexistent.permission")
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+org.Slug+"/rbac/groups/"+group.ID.String()+"/permissions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 unknown permission, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "permission does not exist in organization") {
		t.Fatalf("expected unknown permission message, got: %s", res.Body.String())
	}
}

func TestRBACGroupDeleteCrossOrgRejected(t *testing.T) {
	r, rbacSvc, db, _, setUser := setupWebHandlerTest(t)

	adminA := models.User{Email: "admin-a-rbac-cross@test.local", DisplayName: "admin-a", Source: "local"}
	adminB := models.User{Email: "admin-b-rbac-cross@test.local", DisplayName: "admin-b", Source: "local"}
	if err := db.Create(&adminA).Error; err != nil {
		t.Fatalf("create adminA: %v", err)
	}
	if err := db.Create(&adminB).Error; err != nil {
		t.Fatalf("create adminB: %v", err)
	}
	orgA, err := rbacSvc.CreateOrganizationWithAdmin(adminA, "Org A RBAC Cross")
	if err != nil {
		t.Fatalf("create orgA: %v", err)
	}
	orgB, err := rbacSvc.CreateOrganizationWithAdmin(adminB, "Org B RBAC Cross")
	if err != nil {
		t.Fatalf("create orgB: %v", err)
	}
	group, err := rbacSvc.CreateGroup(orgA.ID, "org-a-group")
	if err != nil {
		t.Fatalf("create group orgA: %v", err)
	}

	setUser(adminB)
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+orgB.Slug+"/rbac/groups/"+group.ID.String()+"/delete", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 deleting cross-org group, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "group not found") {
		t.Fatalf("expected group not found message, got: %s", res.Body.String())
	}
}
