package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arencloud/qdash/internal/kube"
	"github.com/arencloud/qdash/internal/middleware"
	"github.com/arencloud/qdash/internal/models"
	"github.com/arencloud/qdash/internal/rbac"
	"github.com/arencloud/qdash/internal/service"
	"github.com/arencloud/qdash/internal/validation"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Handler struct {
	rbac    *rbac.Service
	authSvc *service.AuthService
	oidcSvc *service.OIDCService
	kubeSvc *service.ResourceService
}

type auditEventRow struct {
	CreatedAt string
	EventType string
	Status    string
	Resource  string
	Message   string
	Actor     string
	Details   string
}

type membershipRow struct {
	UserID      string
	Email       string
	DisplayName string
	Role        string
	Permission  string
}

type groupRow struct {
	ID          string
	Name        string
	Members     []membershipRow
	Permissions []string
}

type resourceKind struct {
	Param           string
	Title           string
	ReadPermission  string
	WritePermission string
	GVR             schema.GroupVersionResource
	FormFields      []string
	GatewayClasses  []string
}

type resourceRow struct {
	Name      string
	CreatedAt string
	Spec      string
}

type namespaceOption struct {
	Name   string
	Exists bool
}

type validationFieldError struct {
	Field   string
	Message string
}

type validationErrors []validationFieldError

func (v *validationErrors) add(field, message string) {
	*v = append(*v, validationFieldError{Field: field, Message: message})
}

func (v validationErrors) any() bool {
	return len(v) > 0
}

func fromValidationFieldErrors(errs []validation.FieldError) validationErrors {
	out := make(validationErrors, 0, len(errs))
	for _, err := range errs {
		out = append(out, validationFieldError{Field: err.Field, Message: err.Message})
	}
	return out
}

func NewHandler(rbacSvc *rbac.Service, authSvc *service.AuthService, oidcSvc *service.OIDCService, kubeSvc *service.ResourceService) *Handler {
	return &Handler{rbac: rbacSvc, authSvc: authSvc, oidcSvc: oidcSvc, kubeSvc: kubeSvc}
}

func (h *Handler) RegisterPublic(r *gin.Engine) {
	r.GET("/login", h.loginPage)
	r.GET("/auth/oidc/start", h.oidcStart)
	r.GET("/auth/oidc/callback", h.oidcCallback)
}

func (h *Handler) RegisterProtected(rg *gin.RouterGroup) {
	rg.GET("/", h.home)
	rg.POST("/logout", h.logout)
	rg.GET("/organizations/new", h.newOrganizationForm)
	rg.POST("/organizations", h.createOrganization)
	rg.GET("/organizations/:slug/settings", h.orgSettings)
	rg.POST("/organizations/:slug/settings/update", h.orgSettingsUpdate)
	rg.GET("/organizations/:slug/rbac", h.orgRBAC)
	rg.GET("/organizations/:slug/rbac/panel", h.orgRBACPanel)
	rg.POST("/organizations/:slug/rbac/users/upsert", h.rbacUpsertMembership)
	rg.POST("/organizations/:slug/rbac/permissions", h.rbacCreatePermission)
	rg.POST("/organizations/:slug/rbac/groups", h.rbacCreateGroup)
	rg.POST("/organizations/:slug/rbac/groups/:groupID/delete", h.rbacDeleteGroup)
	rg.POST("/organizations/:slug/rbac/groups/:groupID/users", h.rbacAddGroupMember)
	rg.POST("/organizations/:slug/rbac/groups/:groupID/users/:userID/delete", h.rbacRemoveGroupMember)
	rg.POST("/organizations/:slug/rbac/groups/:groupID/permissions", h.rbacAddGroupPermission)
	rg.POST("/organizations/:slug/rbac/groups/:groupID/permissions/delete", h.rbacRemoveGroupPermission)
	rg.GET("/organizations/:slug/oidc", h.orgOIDC)
	rg.POST("/organizations/:slug/oidc/config", h.oidcSaveConfig)
	rg.POST("/organizations/:slug/oidc/mappings", h.oidcCreateMapping)
	rg.POST("/organizations/:slug/oidc/mappings/:mappingID/delete", h.oidcDeleteMapping)
	rg.GET("/organizations/:slug/audit", h.orgAudit)
	rg.GET("/organizations/:slug/resources", h.orgResources)
	rg.GET("/organizations/:slug/resources/namespaces/panel", h.resourceNamespacesPanel)
	rg.GET("/organizations/:slug/resources/namespaces/workspace", h.resourceNamespacesWorkspace)
	rg.GET("/organizations/:slug/resources/namespaces/labels", h.resourceNamespaceLabelOptions)
	rg.POST("/organizations/:slug/resources/namespaces/create", h.resourceNamespaceCreate)
	rg.POST("/organizations/:slug/resources/namespaces/adopt", h.resourceNamespaceAdopt)
	rg.GET("/organizations/:slug/resources/:kind/list", h.resourceListPartial)
	rg.GET("/organizations/:slug/resources/:kind/edit", h.resourceEdit)
	rg.POST("/organizations/:slug/resources/:kind/apply", h.resourceApply)
	rg.POST("/organizations/:slug/resources/:kind/delete", h.resourceDelete)
	rg.GET("/resources", h.resources)
}

func (h *Handler) loginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login", gin.H{
		"OIDCEnabled": h.oidcSvc.Enabled(),
		"Error":       c.Query("error"),
	})
}

func (h *Handler) oidcStart(c *gin.Context) {
	if !h.oidcSvc.Enabled() {
		c.Redirect(http.StatusFound, "/login?error=oidc_not_configured")
		return
	}
	url, err := h.oidcSvc.AuthCodeURL()
	if err != nil {
		c.Redirect(http.StatusFound, "/login?error=oidc_start_failed")
		return
	}
	c.Redirect(http.StatusFound, url)
}

func (h *Handler) oidcCallback(c *gin.Context) {
	if c.Query("error") != "" {
		c.Redirect(http.StatusFound, "/login?error=oidc_provider_error")
		return
	}
	identity, err := h.oidcSvc.ExchangeCallback(c.Request.Context(), c.Query("state"), c.Query("code"))
	if err != nil {
		c.Redirect(http.StatusFound, "/login?error=oidc_callback_failed")
		return
	}
	user, err := h.authSvc.EnsureOIDCUser(identity.Email, identity.DisplayName)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?error=user_sync_failed")
		return
	}
	if err := h.rbac.ApplyOIDCMappings(user.ID, identity.RawClaims); err != nil {
		c.Redirect(http.StatusFound, "/login?error=oidc_mapping_failed")
		return
	}
	token, err := h.authSvc.CreateSession(user.ID)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?error=session_failed")
		return
	}
	c.SetCookie(middleware.SessionCookieName, token, int(service.SessionTTL.Seconds()), "/", "", false, true)
	c.Redirect(http.StatusFound, "/")
}

func (h *Handler) logout(c *gin.Context) {
	token, _ := c.Cookie(middleware.SessionCookieName)
	_ = h.authSvc.Logout(token)
	c.SetCookie(middleware.SessionCookieName, "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/login")
}

func (h *Handler) home(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	orgs, _ := h.rbac.ListOrganizationsForUser(user.ID)
	c.HTML(http.StatusOK, "home", gin.H{"User": user, "Organizations": orgs})
}

func (h *Handler) newOrganizationForm(c *gin.Context) {
	c.HTML(http.StatusOK, "org_form", gin.H{})
}

func (h *Handler) createOrganization(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	name := c.PostForm("name")
	if name == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "organization name is required"})
		return
	}
	_, err := h.rbac.CreateOrganizationWithAdmin(user, name)
	if err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	orgs, _ := h.rbac.ListOrganizationsForUser(user.ID)
	c.HTML(http.StatusOK, "org_list", gin.H{"Organizations": orgs})
}

func (h *Handler) orgSettings(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "organizations.read")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	settings := map[string]any{}
	if len(org.Settings) > 0 {
		_ = json.Unmarshal(org.Settings, &settings)
	}
	c.HTML(http.StatusOK, "org_settings", gin.H{
		"Slug":        c.Param("slug"),
		"OrgName":     org.Name,
		"Description": org.Description,
		"Settings":    settings,
	})
}

func (h *Handler) orgSettingsUpdate(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, isAdmin, err := h.rbac.IsOrgAdmin(user.ID, c.Param("slug"))
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	if !isAdmin {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "admin role required"})
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	description := strings.TrimSpace(c.PostForm("description"))
	if err := h.rbac.UpdateOrganization(org.ID, name, description); err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	settings := map[string]any{
		"defaultIstioDiscoveryLabel": strings.TrimSpace(c.PostForm("default_istio_discovery_label")),
		"defaultIstioRevisionTag":    strings.TrimSpace(c.PostForm("default_istio_revision_tag")),
		"contactEmail":               strings.TrimSpace(c.PostForm("contact_email")),
		"environment":                strings.TrimSpace(c.PostForm("environment")),
	}
	if err := h.rbac.UpdateOrganizationSettings(org.ID, settings); err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "organization settings updated"})
}

func (h *Handler) orgRBAC(c *gin.Context) {
	if !h.ensureOrgPermission(c, "rbac.read") {
		return
	}
	c.HTML(http.StatusOK, "org_rbac", gin.H{"Slug": c.Param("slug")})
}

func (h *Handler) orgRBACPanel(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	data, status, err := h.buildRBACViewData(user.ID, c.Param("slug"))
	if err != nil {
		c.HTML(status, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.HTML(http.StatusOK, "rbac_panel", data)
}

func (h *Handler) rbacUpsertMembership(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "rbac.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	email := strings.TrimSpace(c.PostForm("email"))
	role := strings.TrimSpace(c.PostForm("role"))
	if email == "" || role == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "email and role are required"})
		return
	}
	custom := splitCSV(c.PostForm("custom_permissions"))
	if err := h.rbac.UpsertMembership(org.ID, email, role, custom); err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Trigger", "rbacChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "membership updated"})
}

func (h *Handler) rbacCreatePermission(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "rbac.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	resource := strings.TrimSpace(c.PostForm("resource"))
	action := strings.TrimSpace(c.PostForm("action"))
	definition := strings.TrimSpace(c.PostForm("definition"))
	if resource == "" || action == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "resource and action are required"})
		return
	}
	if name == "" {
		name = resource + "." + action
	}
	err = h.rbac.CreatePermission(org.ID, models.Permission{
		Name:       name,
		Resource:   resource,
		Action:     action,
		Definition: definition,
	})
	if err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Trigger", "rbacChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "custom permission created"})
}

func (h *Handler) buildRBACViewData(userID uuid.UUID, slug string) (gin.H, int, error) {
	org, err := h.rbac.Authorize(userID, slug, "rbac.read")
	if err != nil {
		return nil, http.StatusForbidden, errors.New("forbidden")
	}
	memberships, err := h.rbac.ListMemberships(org.ID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.New("failed to load memberships")
	}
	permissions, err := h.rbac.ListPermissions(org.ID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.New("failed to load permissions")
	}
	groups, err := h.rbac.ListGroups(org.ID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.New("failed to load groups")
	}

	userIDs := make([]uuid.UUID, 0, len(memberships))
	for _, m := range memberships {
		userIDs = append(userIDs, m.UserID)
	}
	usersByID, err := h.rbac.GetUsersByIDs(userIDs)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.New("failed to load users")
	}

	memberRows := make([]membershipRow, 0, len(memberships))
	for _, m := range memberships {
		u := usersByID[m.UserID]
		memberRows = append(memberRows, membershipRow{
			UserID:      m.UserID.String(),
			Email:       u.Email,
			DisplayName: u.DisplayName,
			Role:        m.Role,
			Permission:  m.Permission,
		})
	}

	permNames := make([]string, 0, len(permissions))
	for _, p := range permissions {
		permNames = append(permNames, p.Name)
	}
	sort.Strings(permNames)

	groupRows := make([]groupRow, 0, len(groups))
	for _, g := range groups {
		groupMembers, membersErr := h.rbac.ListGroupMembers(org.ID, g.ID)
		if membersErr != nil {
			return nil, http.StatusInternalServerError, errors.New("failed to load group members")
		}
		groupPerms, permsErr := h.rbac.ListGroupPermissions(org.ID, g.ID)
		if permsErr != nil {
			return nil, http.StatusInternalServerError, errors.New("failed to load group permissions")
		}

		members := make([]membershipRow, 0, len(groupMembers))
		for _, gm := range groupMembers {
			members = append(members, membershipRow{
				UserID:      gm.ID.String(),
				Email:       gm.Email,
				DisplayName: gm.DisplayName,
			})
		}
		perms := make([]string, 0, len(groupPerms))
		for _, gp := range groupPerms {
			perms = append(perms, gp.Permission)
		}
		sort.Strings(perms)
		groupRows = append(groupRows, groupRow{
			ID:          g.ID.String(),
			Name:        g.Name,
			Members:     members,
			Permissions: perms,
		})
	}

	return gin.H{
		"Slug":        slug,
		"Memberships": memberRows,
		"Permissions": permNames,
		"Groups":      groupRows,
	}, http.StatusOK, nil
}

func (h *Handler) rbacCreateGroup(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "rbac.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "group name is required"})
		return
	}
	if _, err := h.rbac.CreateGroup(org.ID, name); err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Trigger", "rbacChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "group created"})
}

func (h *Handler) rbacDeleteGroup(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "rbac.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "invalid group id"})
		return
	}
	if err := h.rbac.DeleteGroup(org.ID, groupID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "group not found"})
			return
		}
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Trigger", "rbacChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "group deleted"})
}

func (h *Handler) rbacAddGroupMember(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "rbac.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "invalid group id"})
		return
	}
	email := strings.TrimSpace(c.PostForm("email"))
	if email == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "email is required"})
		return
	}
	if _, err := h.rbac.AddGroupMemberByEmail(org.ID, groupID, email); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "group not found"})
			return
		}
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Trigger", "rbacChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "member added"})
}

func (h *Handler) rbacRemoveGroupMember(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "rbac.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "invalid group id"})
		return
	}
	userID, err := uuid.Parse(c.Param("userID"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "invalid user id"})
		return
	}
	if err := h.rbac.RemoveGroupMember(org.ID, groupID, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "group not found"})
			return
		}
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Trigger", "rbacChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "member removed"})
}

func (h *Handler) rbacAddGroupPermission(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "rbac.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "invalid group id"})
		return
	}
	permission := strings.TrimSpace(c.PostForm("permission"))
	if permission == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "permission is required"})
		return
	}
	if err := h.rbac.AddGroupPermission(org.ID, groupID, permission); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "group not found"})
			return
		}
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Trigger", "rbacChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "permission added"})
}

func (h *Handler) rbacRemoveGroupPermission(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "rbac.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "invalid group id"})
		return
	}
	permission := strings.TrimSpace(c.PostForm("permission"))
	if permission == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "permission is required"})
		return
	}
	if err := h.rbac.RemoveGroupPermission(org.ID, groupID, permission); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "group not found"})
			return
		}
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Trigger", "rbacChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "permission removed"})
}

func (h *Handler) orgOIDC(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "organizations.read")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	cfg, err := h.rbac.GetOIDCConfig(org.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.HTML(http.StatusInternalServerError, "flash", gin.H{"Message": "failed to load OIDC config"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cfg.GroupClaim = "groups"
		cfg.UsernameClaim = "email"
	}
	mappings, err := h.rbac.ListOIDCMapping(org.ID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "flash", gin.H{"Message": "failed to load OIDC mappings"})
		return
	}
	c.HTML(http.StatusOK, "org_oidc", gin.H{
		"Slug":     c.Param("slug"),
		"Config":   cfg,
		"Mappings": mappings,
	})
}

func (h *Handler) oidcSaveConfig(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "organizations.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	cfg := models.OIDCConfig{
		IssuerURL:     strings.TrimSpace(c.PostForm("issuer_url")),
		ClientID:      strings.TrimSpace(c.PostForm("client_id")),
		ClientSecret:  strings.TrimSpace(c.PostForm("client_secret")),
		GroupClaim:    strings.TrimSpace(c.PostForm("group_claim")),
		UsernameClaim: strings.TrimSpace(c.PostForm("username_claim")),
		Enabled:       parseBoolPost(c.PostForm("enabled")),
	}
	if cfg.GroupClaim == "" {
		cfg.GroupClaim = "groups"
	}
	if cfg.UsernameClaim == "" {
		cfg.UsernameClaim = "email"
	}
	if cfg.IssuerURL == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "issuer url, client id and client secret are required"})
		return
	}
	if err := h.rbac.UpsertOIDCConfig(org.ID, cfg); err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Refresh", "true")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "OIDC configuration updated"})
}

func (h *Handler) oidcCreateMapping(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "organizations.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	mapping := models.OIDCMapping{
		SubjectType:      strings.TrimSpace(c.PostForm("subject_type")),
		ExternalValue:    strings.TrimSpace(c.PostForm("external_value")),
		ExternalGroup:    strings.TrimSpace(c.PostForm("external_group")),
		MappedRole:       strings.TrimSpace(c.PostForm("mapped_role")),
		CustomPermission: strings.TrimSpace(c.PostForm("custom_permission")),
	}
	if mapping.ExternalValue == "" {
		mapping.ExternalValue = mapping.ExternalGroup
	}
	if mapping.ExternalValue == "" || mapping.MappedRole == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "external value and role are required"})
		return
	}
	if err := h.rbac.CreateOIDCMapping(org.ID, mapping); err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Refresh", "true")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "OIDC mapping added"})
}

func (h *Handler) oidcDeleteMapping(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "organizations.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	id, err := uuid.Parse(c.Param("mappingID"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "invalid mapping id"})
		return
	}
	if err := h.rbac.DeleteOIDCMapping(org.ID, id); err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.Header("HX-Refresh", "true")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "OIDC mapping deleted"})
}

func (h *Handler) orgAudit(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "organizations.read")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	limit := 200
	if q := strings.TrimSpace(c.Query("limit")); q != "" {
		if n, convErr := strconv.Atoi(q); convErr == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	resourceFilter := strings.TrimSpace(c.Query("resource"))
	statusFilter := strings.TrimSpace(c.Query("status"))
	eventTypeFilter := strings.TrimSpace(c.Query("eventType"))

	events, err := h.rbac.ListAuditEventsFiltered(org.ID, limit, resourceFilter, statusFilter, eventTypeFilter)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "flash", gin.H{"Message": "failed to load audit events"})
		return
	}
	actorIDs := make([]uuid.UUID, 0, len(events))
	for _, e := range events {
		actorIDs = append(actorIDs, e.ActorUserID)
	}
	actors, err := h.rbac.GetUsersByIDs(actorIDs)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "flash", gin.H{"Message": "failed to load audit actors"})
		return
	}
	rows := make([]auditEventRow, 0, len(events))
	for _, e := range events {
		rows = append(rows, auditEventRow{
			CreatedAt: e.CreatedAt.Format("2006-01-02 15:04:05 MST"),
			EventType: e.EventType,
			Status:    e.Status,
			Resource:  e.Resource,
			Message:   e.Message,
			Actor:     actorLabel(actors[e.ActorUserID], e.ActorUserID),
			Details:   prettyJSON(e.Details),
		})
	}
	c.HTML(http.StatusOK, "org_audit", gin.H{
		"Slug": c.Param("slug"),
		"Rows": rows,
		"Filter": gin.H{
			"Resource":  resourceFilter,
			"Status":    statusFilter,
			"EventType": eventTypeFilter,
			"Limit":     limit,
		},
	})
}

func (h *Handler) orgResources(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "organizations.read")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	namespaces, defaultNS, err := h.loadNamespaceOptions(c, org.ID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "flash", gin.H{"Message": err.Error()})
		return
	}
	activeNamespace := strings.TrimSpace(c.Query("namespace"))
	if activeNamespace == "" {
		activeNamespace = defaultNS
	}
	if activeNamespace != "" && !namespaceExistsInOptions(namespaces, activeNamespace) {
		activeNamespace = defaultNS
	}
	activeKind := normalizeResourceWorkspaceKind(c.Query("kind"))
	menuKinds := append([]resourceKind{
		{Param: "namespaces", Title: "Namespaces"},
	}, allResourceKinds()...)
	activeKindTitle := "Namespaces"
	for _, k := range menuKinds {
		if k.Param == activeKind {
			activeKindTitle = k.Title
			break
		}
	}
	view := gin.H{
		"Slug":            c.Param("slug"),
		"MenuKinds":       menuKinds,
		"ActiveKind":      activeKind,
		"ActiveKindTitle": activeKindTitle,
		"ActiveNamespace": activeNamespace,
	}

	if activeKind == "namespaces" {
		view["IsNamespaceWorkspace"] = true
		c.HTML(http.StatusOK, "org_resources", view)
		return
	}

	workspaceKind, ok := resourceKindByParam(activeKind)
	if !ok {
		view["WorkspaceError"] = "unknown resource kind"
		c.HTML(http.StatusOK, "org_resources", view)
		return
	}
	if _, err := h.rbac.Authorize(user.ID, c.Param("slug"), workspaceKind.ReadPermission); err != nil {
		view["WorkspaceError"] = "forbidden for selected resource"
		view["WorkspaceKind"] = workspaceKind
		c.HTML(http.StatusOK, "org_resources", view)
		return
	}

	if workspaceKind.Param == "gateways" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		classes, classErr := h.kubeSvc.ListGatewayClasses(ctx)
		if classErr != nil {
			view["GatewayClassError"] = classErr.Error()
		} else {
			sort.Strings(classes)
			workspaceKind.GatewayClasses = classes
		}
	}
	view["WorkspaceKind"] = workspaceKind
	c.HTML(http.StatusOK, "org_resources", view)
}

func (h *Handler) resourceNamespacesPanel(c *gin.Context) {
	workspace, status, err := h.buildNamespaceWorkspaceData(c)
	if err != nil {
		c.HTML(status, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.HTML(http.StatusOK, "namespace_panel", workspace)
}

func (h *Handler) resourceNamespacesWorkspace(c *gin.Context) {
	workspace, status, err := h.buildNamespaceWorkspaceData(c)
	if err != nil {
		c.HTML(status, "flash", gin.H{"Message": err.Error()})
		return
	}
	c.HTML(http.StatusOK, "namespace_workspace", workspace)
}

func (h *Handler) resourceNamespaceLabelOptions(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	_, err := h.rbac.Authorize(user.ID, c.Param("slug"), "gateway.read")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	instanceConfigs, discoverErr := h.kubeSvc.DiscoverIstioInstanceConfigs(ctx)
	if discoverErr != nil {
		c.HTML(http.StatusBadGateway, "flash", gin.H{"Message": discoverErr.Error()})
		return
	}
	discovery := strings.TrimSpace(c.Query("discovery_label"))
	if discovery == "" {
		discovery = firstDiscovery(instanceConfigs)
	}
	cfg := findIstioInstanceConfig(instanceConfigs, discovery)
	if cfg.DiscoveryLabel == "" && len(instanceConfigs) > 0 {
		cfg = instanceConfigs[0]
	}
	revision := strings.TrimSpace(c.Query("revision_tag"))
	if revision == "" {
		revision = firstOrDefault(cfg.RevisionTags, "default")
	}
	if !containsString(cfg.RevisionTags, revision) {
		revision = firstOrDefault(cfg.RevisionTags, "default")
	}
	c.HTML(http.StatusOK, "namespace_revision_and_labels", gin.H{
		"Slug":            c.Param("slug"),
		"RevisionTags":    cfg.RevisionTags,
		"DefaultRevision": revision,
		"LabelOptions":    cfg.AdditionalLabels,
	})
}

func (h *Handler) buildNamespaceWorkspaceData(c *gin.Context) (gin.H, int, error) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "gateway.read")
	if err != nil {
		return nil, http.StatusForbidden, errors.New("forbidden")
	}
	namespaces, defaultNS, err := h.loadNamespaceOptions(c, org.ID)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	selectedNS := strings.TrimSpace(c.Query("namespace"))
	if selectedNS == "" {
		selectedNS = strings.TrimSpace(c.Query("active_namespace"))
	}
	if selectedNS == "" {
		selectedNS = defaultNS
	}
	if selectedNS != "" && !namespaceExistsInOptions(namespaces, selectedNS) {
		selectedNS = defaultNS
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	instanceConfigs, discoverErr := h.kubeSvc.DiscoverIstioInstanceConfigs(ctx)
	if discoverErr != nil {
		return nil, http.StatusBadGateway, discoverErr
	}
	discoveryLabels := make([]string, 0, len(instanceConfigs))
	for _, cfg := range instanceConfigs {
		discoveryLabels = append(discoveryLabels, cfg.DiscoveryLabel)
	}
	defaultDiscovery := firstOrDefault(discoveryLabels, "default")
	settings := map[string]any{}
	if len(org.Settings) > 0 {
		_ = json.Unmarshal(org.Settings, &settings)
	}
	if v, ok := settings["defaultIstioDiscoveryLabel"].(string); ok && strings.TrimSpace(v) != "" {
		defaultDiscovery = strings.TrimSpace(v)
	}
	selectedConfig := findIstioInstanceConfig(instanceConfigs, defaultDiscovery)
	if selectedConfig.DiscoveryLabel == "" && len(instanceConfigs) > 0 {
		selectedConfig = instanceConfigs[0]
	}
	defaultRevision := firstOrDefault(selectedConfig.RevisionTags, "default")
	if v, ok := settings["defaultIstioRevisionTag"].(string); ok && strings.TrimSpace(v) != "" {
		candidate := strings.TrimSpace(v)
		if containsString(selectedConfig.RevisionTags, candidate) {
			defaultRevision = candidate
		}
	}

	return gin.H{
		"Slug":             c.Param("slug"),
		"Namespaces":       namespaces,
		"SelectedNS":       selectedNS,
		"DiscoveryLabels":  discoveryLabels,
		"RevisionTags":     selectedConfig.RevisionTags,
		"DefaultDiscovery": defaultDiscovery,
		"DefaultRevision":  defaultRevision,
		"LabelOptions":     append([]string{}, selectedConfig.AdditionalLabels...),
	}, http.StatusOK, nil
}

func (h *Handler) resourceNamespaceCreate(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), "gateway.write")
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	namespace := strings.TrimSpace(c.PostForm("name"))
	if namespace == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "namespace name is required"})
		return
	}
	discoveryLabel := strings.TrimSpace(c.PostForm("discovery_label"))
	revisionTag := strings.TrimSpace(c.PostForm("revision_tag"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	instanceConfigs, discoverErr := h.kubeSvc.DiscoverIstioInstanceConfigs(ctx)
	if discoverErr != nil {
		c.HTML(http.StatusBadGateway, "flash", gin.H{"Message": discoverErr.Error()})
		return
	}
	if discoveryLabel == "" || revisionTag == "" {
		settings := map[string]any{}
		if len(org.Settings) > 0 {
			_ = json.Unmarshal(org.Settings, &settings)
		}
		if discoveryLabel == "" {
			if v, ok := settings["defaultIstioDiscoveryLabel"].(string); ok {
				discoveryLabel = strings.TrimSpace(v)
			}
		}
		if revisionTag == "" {
			if v, ok := settings["defaultIstioRevisionTag"].(string); ok {
				revisionTag = strings.TrimSpace(v)
			}
		}
	}
	if discoveryLabel == "" {
		discoveryLabel = firstDiscovery(instanceConfigs)
	}
	discoveryLabel = normalizeDiscoverySelection(discoveryLabel)
	cfg := findIstioInstanceConfig(instanceConfigs, discoveryLabel)
	if cfg.DiscoveryLabel == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "invalid istio discovery label for selected istiod instance"})
		return
	}
	if revisionTag == "" {
		revisionTag = firstOrDefault(cfg.RevisionTags, "")
	}
	if revisionTag == "" || !containsString(cfg.RevisionTags, revisionTag) {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "invalid revision/tag for selected istiod instance"})
		return
	}
	if discoveryLabel == "" || revisionTag == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "discovery label and revision tag are required"})
		return
	}
	discoveryKey, discoveryValue, discoveryErr := splitLabelKV(discoveryLabel)
	if discoveryErr != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "invalid discovery label format, expected key=value"})
		return
	}

	allowedLabels := map[string]bool{}
	for _, opt := range cfg.AdditionalLabels {
		allowedLabels[strings.TrimSpace(opt)] = true
	}
	labels := map[string]string{
		discoveryKey:   discoveryValue,
		"istio.io/rev": revisionTag,
	}
	for _, kv := range c.PostFormArray("labels") {
		normalized := strings.TrimSpace(kv)
		if normalized == "" {
			continue
		}
		if !allowedLabels[normalized] {
			c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "additional labels must be selected from chosen istiod instance config"})
			return
		}
		parts := strings.SplitN(normalized, "=", 2)
		if len(parts) != 2 {
			continue
		}
		labels[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	if err := h.kubeSvc.CreateNamespace(ctx, namespace, labels); err != nil {
		_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "namespace.create.failed", "namespace", "failed", "namespace creation failed", map[string]any{
			"namespace":      namespace,
			"discoveryLabel": discoveryLabel,
			"revisionTag":    revisionTag,
			"error":          err.Error(),
		})
		c.HTML(http.StatusBadGateway, "flash", gin.H{"Message": err.Error()})
		return
	}
	if err := h.rbac.ClaimNamespace(org.ID, user.ID, namespace); err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "namespace.create", "namespace", "success", "namespace created and claimed", map[string]any{
		"namespace":      namespace,
		"discoveryLabel": discoveryLabel,
		"revisionTag":    revisionTag,
	})
	c.Header("HX-Trigger", "namespaceChanged,resourceChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "namespace created and linked to organization"})
}

func (h *Handler) resourceNamespaceAdopt(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	org, isAdmin, err := h.rbac.IsOrgAdmin(user.ID, c.Param("slug"))
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	if !isAdmin {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "admin role required"})
		return
	}
	namespace := strings.TrimSpace(c.PostForm("name"))
	if namespace == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "namespace name is required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	exists, err := h.kubeSvc.NamespaceExists(ctx, namespace)
	if err != nil {
		c.HTML(http.StatusBadGateway, "flash", gin.H{"Message": err.Error()})
		return
	}
	if !exists {
		c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "namespace not found in cluster"})
		return
	}
	if err := h.rbac.ClaimNamespace(org.ID, user.ID, namespace); err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "namespace.adopt", "namespace", "success", "existing namespace adopted", map[string]any{
		"namespace": namespace,
	})
	c.Header("HX-Trigger", "namespaceChanged,resourceChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": "namespace adopted"})
}

func (h *Handler) resourceListPartial(c *gin.Context) {
	cfg, ok := resourceKindByParam(c.Param("kind"))
	if !ok {
		c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "unknown resource kind"})
		return
	}
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), cfg.ReadPermission)
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	if namespace == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "select namespace"})
		return
	}
	owned, err := h.rbac.NamespaceBelongsToOrg(org.ID, namespace)
	if err != nil || !owned {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "namespace is not owned by organization"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	items, err := h.kubeSvc.List(ctx, cfg.GVR, namespace)
	if err != nil {
		c.HTML(http.StatusBadGateway, "flash", gin.H{"Message": err.Error()})
		return
	}
	rows := make([]resourceRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, resourceRow{
			Name:      item.GetName(),
			CreatedAt: item.GetCreationTimestamp().Format("2006-01-02 15:04:05 MST"),
			Spec:      prettyObject(item.Object["spec"]),
		})
	}
	c.HTML(http.StatusOK, "resource_list", gin.H{
		"Rows":       rows,
		"Kind":       cfg.Title,
		"KindParam":  cfg.Param,
		"Slug":       c.Param("slug"),
		"Namespace":  namespace,
		"FeedbackID": "resource-feedback-" + cfg.Param,
	})
}

func (h *Handler) resourceApply(c *gin.Context) {
	cfg, ok := resourceKindByParam(c.Param("kind"))
	if !ok {
		c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "unknown resource kind"})
		return
	}
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), cfg.WritePermission)
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	namespace := strings.TrimSpace(c.PostForm("namespace"))
	name := strings.TrimSpace(c.PostForm("name"))
	if namespace == "" || name == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "namespace and name are required"})
		return
	}
	owned, err := h.rbac.NamespaceBelongsToOrg(org.ID, namespace)
	if err != nil || !owned {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "namespace is not owned by organization"})
		return
	}
	spec, fieldErrs, err := buildSpecFromForm(cfg.Param, c)
	if err != nil {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": err.Error()})
		return
	}
	if fieldErrs.any() {
		c.HTML(http.StatusBadRequest, "validation_errors", gin.H{"Errors": fieldErrs})
		return
	}
	if semanticErrs := validation.ValidateResourceSpec(cfg.Param, spec); len(semanticErrs) > 0 {
		c.HTML(http.StatusBadRequest, "validation_errors", gin.H{"Errors": fromValidationFieldErrors(semanticErrs)})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if err := h.kubeSvc.UpsertGeneric(ctx, cfg.GVR, namespace, name, spec); err != nil {
		_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "resource.apply.failed", cfg.Param, "failed", "resource apply failed", map[string]any{
			"namespace": namespace,
			"name":      name,
			"error":     err.Error(),
		})
		c.HTML(http.StatusBadGateway, "flash", gin.H{"Message": err.Error()})
		return
	}
	_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "resource.apply", cfg.Param, "success", "resource applied", map[string]any{
		"namespace": namespace,
		"name":      name,
	})
	c.Header("HX-Trigger", "resourceChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": fmt.Sprintf("%s/%s applied in %s", cfg.Title, name, namespace)})
}

func (h *Handler) resourceEdit(c *gin.Context) {
	cfg, ok := resourceKindByParam(c.Param("kind"))
	if !ok {
		c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "unknown resource kind"})
		return
	}
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), cfg.ReadPermission)
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	name := strings.TrimSpace(c.Query("name"))
	if namespace == "" || name == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "namespace and name are required"})
		return
	}
	owned, err := h.rbac.NamespaceBelongsToOrg(org.ID, namespace)
	if err != nil || !owned {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "namespace is not owned by organization"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	items, err := h.kubeSvc.List(ctx, cfg.GVR, namespace)
	if err != nil {
		c.HTML(http.StatusBadGateway, "flash", gin.H{"Message": err.Error()})
		return
	}
	var target *unstructured.Unstructured
	for i := range items {
		if items[i].GetName() == name {
			target = &items[i]
			break
		}
	}
	if target == nil {
		c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "resource not found"})
		return
	}
	specObj, _ := target.Object["spec"]
	advanced := prettyObject(specObj)
	fieldValues := extractFieldValues(cfg.Param, target.Object)
	c.HTML(http.StatusOK, "resource_form_fill", gin.H{
		"KindParam":    cfg.Param,
		"Name":         name,
		"AdvancedJSON": advanced,
		"FieldValues":  fieldValues,
	})
}

func (h *Handler) resourceDelete(c *gin.Context) {
	cfg, ok := resourceKindByParam(c.Param("kind"))
	if !ok {
		c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "unknown resource kind"})
		return
	}
	user, _ := middleware.UserFromContext(c)
	org, err := h.rbac.Authorize(user.ID, c.Param("slug"), cfg.WritePermission)
	if err != nil {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
		return
	}
	namespace := strings.TrimSpace(c.PostForm("namespace"))
	name := strings.TrimSpace(c.PostForm("name"))
	if namespace == "" || name == "" {
		c.HTML(http.StatusBadRequest, "flash", gin.H{"Message": "namespace and name are required"})
		return
	}
	owned, err := h.rbac.NamespaceBelongsToOrg(org.ID, namespace)
	if err != nil || !owned {
		c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "namespace is not owned by organization"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if err := h.kubeSvc.Delete(ctx, cfg.GVR, namespace, name); err != nil {
		_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "resource.delete.failed", cfg.Param, "failed", "resource delete failed", map[string]any{
			"namespace": namespace,
			"name":      name,
			"error":     err.Error(),
		})
		c.HTML(http.StatusBadGateway, "flash", gin.H{"Message": err.Error()})
		return
	}
	_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "resource.delete", cfg.Param, "success", "resource deleted", map[string]any{
		"namespace": namespace,
		"name":      name,
	})
	c.Header("HX-Trigger", "resourceChanged")
	c.HTML(http.StatusOK, "flash", gin.H{"Message": fmt.Sprintf("%s/%s deleted from %s", cfg.Title, name, namespace)})
}

func (h *Handler) resources(c *gin.Context) {
	c.HTML(http.StatusOK, "resources", gin.H{})
}

func (h *Handler) ensureOrgPermission(c *gin.Context, permission string) bool {
	user, _ := middleware.UserFromContext(c)
	_, err := h.rbac.Authorize(user.ID, c.Param("slug"), permission)
	if err == nil {
		return true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.HTML(http.StatusNotFound, "flash", gin.H{"Message": "organization not found or no membership"})
		return false
	}
	c.HTML(http.StatusForbidden, "flash", gin.H{"Message": "forbidden"})
	return false
}

func actorLabel(user models.User, id uuid.UUID) string {
	if user.ID == uuid.Nil {
		return id.String()
	}
	if user.DisplayName != "" && user.Email != "" {
		return fmt.Sprintf("%s (%s)", user.DisplayName, user.Email)
	}
	if user.Email != "" {
		return user.Email
	}
	return id.String()
}

func prettyJSON(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	var js any
	if err := json.Unmarshal(raw, &js); err != nil {
		return string(raw)
	}
	out := &bytes.Buffer{}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(js); err != nil {
		return string(raw)
	}
	return out.String()
}

func prettyObject(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseBoolPost(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func containsString(values []string, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	for _, v := range values {
		if strings.TrimSpace(v) == candidate {
			return true
		}
	}
	return false
}

func firstOrDefault(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return values[0]
}

func firstDiscovery(configs []kube.IstioInstanceConfig) string {
	if len(configs) == 0 {
		return "default"
	}
	return configs[0].DiscoveryLabel
}

func findIstioInstanceConfig(configs []kube.IstioInstanceConfig, discovery string) kube.IstioInstanceConfig {
	discovery = strings.TrimSpace(discovery)
	for _, cfg := range configs {
		if strings.TrimSpace(cfg.DiscoveryLabel) == discovery {
			return cfg
		}
	}
	return kube.IstioInstanceConfig{}
}

func normalizeDiscoverySelection(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if strings.Contains(v, "=") {
		return v
	}
	return "istio-discovery=" + v
}

func normalizeResourceWorkspaceKind(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "namespaces"
	}
	if v == "namespaces" {
		return v
	}
	if _, ok := resourceKindByParam(v); ok {
		return v
	}
	return "namespaces"
}

func splitLabelKV(v string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(v), "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid label format")
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if key == "" || value == "" {
		return "", "", fmt.Errorf("invalid label format")
	}
	return key, value, nil
}

func namespaceLabelOptionsForInstance(instance string) []string {
	profiles := service.NamespaceProfiles()
	sort.Strings(profiles)
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, profile := range profiles {
		labels, err := kube.BuildNamespaceLabels(instance, profile)
		if err != nil {
			continue
		}
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			value := k + "=" + labels[k]
			if seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func resourceKindByParam(param string) (resourceKind, bool) {
	for _, k := range allResourceKinds() {
		if k.Param == param {
			return k, true
		}
	}
	return resourceKind{}, false
}

func allResourceKinds() []resourceKind {
	return []resourceKind{
		{Param: "gateways", Title: "Gateway", ReadPermission: "gateway.read", WritePermission: "gateway.write", GVR: kube.GatewayGVR(), FormFields: []string{"gateway_class", "gateway_class_custom", "listener_name", "protocol", "port", "hostname"}},
		{Param: "httproutes", Title: "HTTPRoute", ReadPermission: "gateway.read", WritePermission: "gateway.write", GVR: kube.HTTPRouteGVR(), FormFields: []string{"parent_gateway", "hostnames", "path_prefix", "backend_service", "backend_port"}},
		{Param: "authpolicies", Title: "AuthPolicy", ReadPermission: "security.read", WritePermission: "security.write", GVR: kube.AuthPolicyGVR(), FormFields: []string{"target_kind", "target_name", "auth_rules_json"}},
		{Param: "ratelimitpolicies", Title: "RateLimitPolicy", ReadPermission: "security.read", WritePermission: "security.write", GVR: kube.RateLimitPolicyGVR(), FormFields: []string{"target_kind", "target_name", "limit_name", "limit_count", "limit_window"}},
	}
}

func toRows(items []unstructured.Unstructured) []resourceRow {
	rows := make([]resourceRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, resourceRow{
			Name:      item.GetName(),
			CreatedAt: item.GetCreationTimestamp().Format("2006-01-02 15:04:05 MST"),
			Spec:      prettyObject(item.Object["spec"]),
		})
	}
	return rows
}

func buildSpecFromForm(kind string, c *gin.Context) (map[string]any, validationErrors, error) {
	advanced := strings.TrimSpace(c.PostForm("spec_json"))
	if advanced != "" {
		var spec map[string]any
		if err := json.Unmarshal([]byte(advanced), &spec); err != nil {
			return nil, nil, fmt.Errorf("invalid advanced JSON spec")
		}
		return spec, nil, nil
	}

	switch kind {
	case "gateways":
		return buildGatewaySpec(c)
	case "httproutes":
		return buildHTTPRouteSpec(c)
	case "authpolicies":
		return buildAuthPolicySpec(c)
	case "ratelimitpolicies":
		return buildRateLimitPolicySpec(c)
	default:
		return nil, nil, fmt.Errorf("unsupported resource kind")
	}
}

func buildGatewaySpec(c *gin.Context) (map[string]any, validationErrors, error) {
	gatewayClass := strings.TrimSpace(c.PostForm("gateway_class"))
	if customClass := strings.TrimSpace(c.PostForm("gateway_class_custom")); customClass != "" {
		gatewayClass = customClass
	}
	errs := validationErrors{}
	if gatewayClass == "" {
		errs.add("gateway_class", "is required")
	}

	names := gatewayFieldArray(c, "listener_name[]", "listener_name")
	protocols := gatewayFieldArray(c, "listener_protocol[]", "protocol")
	ports := gatewayFieldArray(c, "listener_port[]", "port")
	hostnames := gatewayFieldArray(c, "listener_hostname[]", "hostname")
	certNames := gatewayFieldArray(c, "listener_cert_name[]", "")
	certKinds := gatewayFieldArray(c, "listener_cert_kind[]", "")
	certGroups := gatewayFieldArray(c, "listener_cert_group[]", "")

	maxItems := maxLen(names, protocols, ports, hostnames, certNames, certKinds, certGroups)
	listeners := make([]any, 0, maxItems)
	for i := 0; i < maxItems; i++ {
		name := at(names, i)
		protocol := strings.ToUpper(at(protocols, i))
		portRaw := at(ports, i)
		hostname := at(hostnames, i)
		certName := at(certNames, i)
		certKind := at(certKinds, i)
		certGroup := at(certGroups, i)

		if name == "" && protocol == "" && portRaw == "" && hostname == "" && certName == "" {
			continue
		}
		if name == "" {
			errs.add(fmt.Sprintf("listeners[%d].name", i), "is required")
		}
		if protocol == "" {
			errs.add(fmt.Sprintf("listeners[%d].protocol", i), "is required")
		}
		port, portErr := parseIntField(portRaw, "port")
		if portErr != "" {
			errs.add(fmt.Sprintf("listeners[%d].port", i), portErr)
		}

		listener := map[string]any{
			"name":     name,
			"port":     port,
			"protocol": protocol,
		}
		if hostname != "" {
			listener["hostname"] = hostname
		}
		if protocol == "HTTPS" || protocol == "TLS" {
			if certName == "" {
				errs.add(fmt.Sprintf("listeners[%d].certificate", i), "certificate name is required for HTTPS/TLS")
			} else {
				if certKind == "" {
					certKind = "Secret"
				}
				certRef := map[string]any{
					"name": certName,
					"kind": certKind,
				}
				if certGroup != "" {
					certRef["group"] = certGroup
				}
				listener["tls"] = map[string]any{
					"mode":            "Terminate",
					"certificateRefs": []any{certRef},
				}
			}
		}
		listeners = append(listeners, listener)
	}

	if len(listeners) == 0 {
		errs.add("listeners", "at least one listener is required")
	}
	if errs.any() {
		return nil, errs, nil
	}
	return map[string]any{
		"gatewayClassName": gatewayClass,
		"listeners":        listeners,
	}, nil, nil
}

func gatewayFieldArray(c *gin.Context, key, fallback string) []string {
	values := c.PostFormArray(key)
	if len(values) == 0 && fallback != "" {
		if single := strings.TrimSpace(c.PostForm(fallback)); single != "" {
			return []string{single}
		}
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, strings.TrimSpace(v))
	}
	return out
}

func at(values []string, i int) string {
	if i < 0 || i >= len(values) {
		return ""
	}
	return strings.TrimSpace(values[i])
}

func maxLen(values ...[]string) int {
	max := 0
	for _, arr := range values {
		if len(arr) > max {
			max = len(arr)
		}
	}
	return max
}

func buildHTTPRouteSpec(c *gin.Context) (map[string]any, validationErrors, error) {
	parentGateway := strings.TrimSpace(c.PostForm("parent_gateway"))
	pathPrefix := strings.TrimSpace(c.PostForm("path_prefix"))
	backendService := strings.TrimSpace(c.PostForm("backend_service"))
	backendPort, portErr := parseIntField(c.PostForm("backend_port"), "backend_port")
	errs := validationErrors{}
	if parentGateway == "" {
		errs.add("parent_gateway", "is required")
	}
	if backendService == "" {
		errs.add("backend_service", "is required")
	}
	if portErr != "" {
		errs.add("backend_port", portErr)
	}
	if errs.any() {
		return nil, errs, nil
	}
	if pathPrefix == "" {
		pathPrefix = "/"
	}

	spec := map[string]any{
		"parentRefs": []any{
			map[string]any{
				"group": "gateway.networking.k8s.io",
				"kind":  "Gateway",
				"name":  parentGateway,
			},
		},
		"rules": []any{
			map[string]any{
				"matches": []any{
					map[string]any{
						"path": map[string]any{"type": "PathPrefix", "value": pathPrefix},
					},
				},
				"backendRefs": []any{
					map[string]any{"name": backendService, "port": backendPort},
				},
			},
		},
	}
	hostnames := splitCSV(c.PostForm("hostnames"))
	if len(hostnames) > 0 {
		values := make([]any, 0, len(hostnames))
		for _, h := range hostnames {
			values = append(values, h)
		}
		spec["hostnames"] = values
	}
	return spec, nil, nil
}

func buildAuthPolicySpec(c *gin.Context) (map[string]any, validationErrors, error) {
	targetKind := strings.TrimSpace(c.PostForm("target_kind"))
	targetName := strings.TrimSpace(c.PostForm("target_name"))
	if targetKind == "" {
		targetKind = "HTTPRoute"
	}
	if targetName == "" {
		return nil, validationErrors{{Field: "target_name", Message: "is required"}}, nil
	}

	spec := map[string]any{
		"targetRef": map[string]any{
			"group": "gateway.networking.k8s.io",
			"kind":  targetKind,
			"name":  targetName,
		},
	}
	rulesJSON := strings.TrimSpace(c.PostForm("auth_rules_json"))
	if rulesJSON != "" {
		var rules map[string]any
		if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
			return nil, validationErrors{{Field: "auth_rules_json", Message: "must be valid JSON"}}, nil
		}
		spec["rules"] = rules
	}
	return spec, nil, nil
}

func buildRateLimitPolicySpec(c *gin.Context) (map[string]any, validationErrors, error) {
	targetKind := strings.TrimSpace(c.PostForm("target_kind"))
	targetName := strings.TrimSpace(c.PostForm("target_name"))
	limitName := strings.TrimSpace(c.PostForm("limit_name"))
	window := strings.TrimSpace(c.PostForm("limit_window"))
	limit, countErr := parseIntField(c.PostForm("limit_count"), "limit_count")
	if targetKind == "" {
		targetKind = "HTTPRoute"
	}
	errs := validationErrors{}
	if targetName == "" {
		errs.add("target_name", "is required")
	}
	if limitName == "" {
		errs.add("limit_name", "is required")
	}
	if window == "" {
		errs.add("limit_window", "is required")
	}
	if countErr != "" {
		errs.add("limit_count", countErr)
	}
	if errs.any() {
		return nil, errs, nil
	}
	spec := map[string]any{
		"targetRef": map[string]any{
			"group": "gateway.networking.k8s.io",
			"kind":  targetKind,
			"name":  targetName,
		},
		"limits": map[string]any{
			limitName: map[string]any{
				"rates": []any{
					map[string]any{
						"limit":  limit,
						"window": window,
					},
				},
			},
		},
	}
	return spec, nil, nil
}

func parseIntField(raw, name string) (int, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, "is required"
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, "must be an integer"
	}
	if name == "port" || name == "backend_port" {
		if v < 1 || v > 65535 {
			return 0, "must be between 1 and 65535"
		}
	}
	if name == "limit_count" && v <= 0 {
		return 0, "must be greater than 0"
	}
	return v, ""
}

func extractFieldValues(kind string, obj map[string]any) map[string]string {
	spec, _ := obj["spec"].(map[string]any)
	out := map[string]string{}
	if spec == nil {
		return out
	}
	switch kind {
	case "gateways":
		out["gateway_class"] = asString(spec["gatewayClassName"])
		out["gateway_class_custom"] = asString(spec["gatewayClassName"])
		listeners := asSlice(spec["listeners"])
		if len(listeners) > 0 {
			out["listeners_json"] = prettyObject(listeners)
		}
		if len(listeners) > 0 {
			l0, _ := listeners[0].(map[string]any)
			out["listener_name"] = asString(l0["name"])
			out["protocol"] = asString(l0["protocol"])
			out["port"] = asIntString(l0["port"])
			out["hostname"] = asString(l0["hostname"])
		}
	case "httproutes":
		hostnames := asSlice(spec["hostnames"])
		if len(hostnames) > 0 {
			values := make([]string, 0, len(hostnames))
			for _, h := range hostnames {
				values = append(values, asString(h))
			}
			out["hostnames"] = strings.Join(values, ",")
		}
		parents := asSlice(spec["parentRefs"])
		if len(parents) > 0 {
			p0, _ := parents[0].(map[string]any)
			out["parent_gateway"] = asString(p0["name"])
		}
		rules := asSlice(spec["rules"])
		if len(rules) > 0 {
			r0, _ := rules[0].(map[string]any)
			matches := asSlice(r0["matches"])
			if len(matches) > 0 {
				m0, _ := matches[0].(map[string]any)
				path, _ := m0["path"].(map[string]any)
				out["path_prefix"] = asString(path["value"])
			}
			backs := asSlice(r0["backendRefs"])
			if len(backs) > 0 {
				b0, _ := backs[0].(map[string]any)
				out["backend_service"] = asString(b0["name"])
				out["backend_port"] = asIntString(b0["port"])
			}
		}
	case "authpolicies":
		target, _ := spec["targetRef"].(map[string]any)
		out["target_kind"] = asString(target["kind"])
		out["target_name"] = asString(target["name"])
		if rules, ok := spec["rules"]; ok {
			out["auth_rules_json"] = prettyObject(rules)
		}
	case "ratelimitpolicies":
		target, _ := spec["targetRef"].(map[string]any)
		out["target_kind"] = asString(target["kind"])
		out["target_name"] = asString(target["name"])
		limits, _ := spec["limits"].(map[string]any)
		for k, v := range limits {
			out["limit_name"] = k
			limitDef, _ := v.(map[string]any)
			rates := asSlice(limitDef["rates"])
			if len(rates) > 0 {
				r0, _ := rates[0].(map[string]any)
				out["limit_count"] = asIntString(r0["limit"])
				out["limit_window"] = asString(r0["window"])
			}
			break
		}
	}
	return out
}

func asSlice(v any) []any {
	switch vv := v.(type) {
	case []any:
		return vv
	default:
		return nil
	}
}

func asString(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	default:
		return ""
	}
}

func asIntString(v any) string {
	switch vv := v.(type) {
	case int:
		return strconv.Itoa(vv)
	case int32:
		return strconv.Itoa(int(vv))
	case int64:
		return strconv.FormatInt(vv, 10)
	case float64:
		return strconv.Itoa(int(vv))
	case json.Number:
		return vv.String()
	default:
		return ""
	}
}

func (h *Handler) loadNamespaceOptions(c *gin.Context, orgID uuid.UUID) ([]namespaceOption, string, error) {
	ns, err := h.rbac.ListOrgNamespaces(orgID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load namespaces")
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	namespaces := make([]namespaceOption, 0, len(ns))
	for _, v := range ns {
		exists, existsErr := h.kubeSvc.NamespaceExists(ctx, v.Namespace)
		if existsErr != nil {
			exists = false
		}
		namespaces = append(namespaces, namespaceOption{Name: v.Namespace, Exists: exists})
	}
	sort.Slice(namespaces, func(i, j int) bool { return namespaces[i].Name < namespaces[j].Name })
	defaultNS := ""
	if len(namespaces) > 0 {
		defaultNS = namespaces[0].Name
	}
	return namespaces, defaultNS, nil
}

func namespaceExistsInOptions(options []namespaceOption, namespace string) bool {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return false
	}
	for _, option := range options {
		if option.Name == namespace && option.Exists {
			return true
		}
	}
	return false
}
