package rbac

import (
	"errors"
	"fmt"
	"testing"

	"github.com/arencloud/qdash/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
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
	return NewService(db), db
}

func TestNamespaceOwnershipIsolation(t *testing.T) {
	svc, _ := setupTestService(t)

	admin1 := models.User{Email: "admin1@example.com", DisplayName: "admin1", Source: "local"}
	admin2 := models.User{Email: "admin2@example.com", DisplayName: "admin2", Source: "local"}
	if err := svc.db.Create(&admin1).Error; err != nil {
		t.Fatalf("create admin1: %v", err)
	}
	if err := svc.db.Create(&admin2).Error; err != nil {
		t.Fatalf("create admin2: %v", err)
	}

	org1, err := svc.CreateOrganizationWithAdmin(admin1, "Org One")
	if err != nil {
		t.Fatalf("create org1: %v", err)
	}
	org2, err := svc.CreateOrganizationWithAdmin(admin2, "Org Two")
	if err != nil {
		t.Fatalf("create org2: %v", err)
	}

	if err := svc.ClaimNamespace(org1.ID, admin1.ID, "team-a"); err != nil {
		t.Fatalf("claim namespace for org1: %v", err)
	}
	if err := svc.ClaimNamespace(org2.ID, admin2.ID, "team-a"); !errors.Is(err, ErrNamespaceOwnedByAnotherOrg) {
		t.Fatalf("expected ErrNamespaceOwnedByAnotherOrg, got: %v", err)
	}

	belongs, err := svc.NamespaceBelongsToOrg(org1.ID, "team-a")
	if err != nil || !belongs {
		t.Fatalf("expected namespace to belong to org1")
	}
	belongs, err = svc.NamespaceBelongsToOrg(org2.ID, "team-a")
	if err != nil {
		t.Fatalf("check org2 namespace ownership: %v", err)
	}
	if belongs {
		t.Fatalf("expected namespace not to belong to org2")
	}
}

func TestIsOrgAdmin(t *testing.T) {
	svc, _ := setupTestService(t)

	admin := models.User{Email: "admin@example.com", DisplayName: "admin", Source: "local"}
	if err := svc.db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	org, err := svc.CreateOrganizationWithAdmin(admin, "Admin Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	editorEmail := "editor@example.com"
	if err := svc.UpsertMembership(org.ID, editorEmail, "editor", nil); err != nil {
		t.Fatalf("upsert editor membership: %v", err)
	}

	_, isAdmin, err := svc.IsOrgAdmin(admin.ID, org.Slug)
	if err != nil || !isAdmin {
		t.Fatalf("expected admin user to be admin")
	}

	var editor models.User
	if err := svc.db.Where("email = ?", editorEmail).First(&editor).Error; err != nil {
		t.Fatalf("load editor user: %v", err)
	}
	_, isAdmin, err = svc.IsOrgAdmin(editor.ID, org.Slug)
	if err != nil {
		t.Fatalf("check editor admin: %v", err)
	}
	if isAdmin {
		t.Fatalf("expected editor not to be admin")
	}
}

func TestAuthorizeDeniesCrossOrgAccess(t *testing.T) {
	svc, _ := setupTestService(t)

	admin := models.User{Email: "admin@example.com", DisplayName: "admin", Source: "local"}
	outsider := models.User{Email: "outsider@example.com", DisplayName: "outsider", Source: "local"}
	if err := svc.db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := svc.db.Create(&outsider).Error; err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	org, err := svc.CreateOrganizationWithAdmin(admin, "Secure Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	_, err = svc.Authorize(outsider.ID, org.Slug, "organizations.read")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound for outsider access, got: %v", err)
	}
}

func TestApplyOIDCMappingsOnlyMatchingOrg(t *testing.T) {
	svc, db := setupTestService(t)

	user := models.User{Email: "user@example.com", DisplayName: "user", Source: "oidc"}
	admin := models.User{Email: "admin@example.com", DisplayName: "admin", Source: "local"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}

	orgA, err := svc.CreateOrganizationWithAdmin(admin, "Team A")
	if err != nil {
		t.Fatalf("create orgA: %v", err)
	}
	orgB, err := svc.CreateOrganizationWithAdmin(admin, "Team B")
	if err != nil {
		t.Fatalf("create orgB: %v", err)
	}

	if err := svc.UpsertOIDCConfig(orgA.ID, models.OIDCConfig{IssuerURL: "https://issuer", ClientID: "a", ClientSecret: "s", GroupClaim: "groups", UsernameClaim: "email", Enabled: true}); err != nil {
		t.Fatalf("upsert oidc config A: %v", err)
	}
	if err := svc.UpsertOIDCConfig(orgB.ID, models.OIDCConfig{IssuerURL: "https://issuer", ClientID: "b", ClientSecret: "s", GroupClaim: "groups", UsernameClaim: "email", Enabled: true}); err != nil {
		t.Fatalf("upsert oidc config B: %v", err)
	}
	if err := svc.CreateOIDCMapping(orgA.ID, models.OIDCMapping{ExternalGroup: "grp-a", MappedRole: "editor"}); err != nil {
		t.Fatalf("create oidc mapping A: %v", err)
	}
	if err := svc.CreateOIDCMapping(orgB.ID, models.OIDCMapping{ExternalGroup: "grp-b", MappedRole: "viewer"}); err != nil {
		t.Fatalf("create oidc mapping B: %v", err)
	}

	if err := svc.ApplyOIDCMappings(user.ID, map[string]any{"groups": []any{"grp-a"}}); err != nil {
		t.Fatalf("apply mappings: %v", err)
	}

	var mA models.Membership
	if err := db.Where("org_id = ? AND user_id = ?", orgA.ID, user.ID).First(&mA).Error; err != nil {
		t.Fatalf("expected membership in orgA: %v", err)
	}
	if mA.Role != "editor" {
		t.Fatalf("expected orgA role editor, got %s", mA.Role)
	}

	var countB int64
	if err := db.Model(&models.Membership{}).Where("org_id = ? AND user_id = ?", orgB.ID, user.ID).Count(&countB).Error; err != nil {
		t.Fatalf("count orgB membership: %v", err)
	}
	if countB != 0 {
		t.Fatalf("expected no membership in orgB, got %d", countB)
	}
}

func TestApplyOIDCMappingsSupportsUserAndRoleSubjects(t *testing.T) {
	svc, db := setupTestService(t)

	user := models.User{Email: "user2@example.com", DisplayName: "user2", Source: "oidc"}
	admin := models.User{Email: "admin2@example.com", DisplayName: "admin2", Source: "local"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}

	org, err := svc.CreateOrganizationWithAdmin(admin, "Team Subject")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := svc.UpsertOIDCConfig(org.ID, models.OIDCConfig{IssuerURL: "https://issuer", ClientID: "qdash", ClientSecret: "s", GroupClaim: "groups", UsernameClaim: "email", Enabled: true}); err != nil {
		t.Fatalf("upsert oidc config: %v", err)
	}
	if err := svc.CreateOIDCMapping(org.ID, models.OIDCMapping{SubjectType: "user", ExternalValue: "user2@example.com", MappedRole: "editor"}); err != nil {
		t.Fatalf("create oidc mapping user: %v", err)
	}
	if err := svc.CreateOIDCMapping(org.ID, models.OIDCMapping{SubjectType: "role", ExternalValue: "platform-admin", MappedRole: "admin"}); err != nil {
		t.Fatalf("create oidc mapping role: %v", err)
	}

	claims := map[string]any{
		"email": "user2@example.com",
		"roles": []any{"platform-admin"},
	}
	if err := svc.ApplyOIDCMappings(user.ID, claims); err != nil {
		t.Fatalf("apply mappings: %v", err)
	}

	var m models.Membership
	if err := db.Where("org_id = ? AND user_id = ?", org.ID, user.ID).First(&m).Error; err != nil {
		t.Fatalf("expected membership in org: %v", err)
	}
	if m.Role != "admin" {
		t.Fatalf("expected resolved strongest role admin, got %s", m.Role)
	}
}
