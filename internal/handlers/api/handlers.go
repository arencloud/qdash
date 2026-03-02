package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/egevorky/qdash/internal/kube"
	"github.com/egevorky/qdash/internal/middleware"
	"github.com/egevorky/qdash/internal/models"
	"github.com/egevorky/qdash/internal/rbac"
	"github.com/egevorky/qdash/internal/service"
	"github.com/egevorky/qdash/internal/validation"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Handler struct {
	db      *gorm.DB
	rbac    *rbac.Service
	kubeSvc *service.ResourceService
}

func NewHandler(db *gorm.DB, rbacSvc *rbac.Service, kubeSvc *service.ResourceService) *Handler {
	return &Handler{db: db, rbac: rbacSvc, kubeSvc: kubeSvc}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/me", h.me)
	rg.GET("/organizations", h.listOrganizations)
	rg.POST("/organizations", h.createOrganization)

	org := rg.Group("/orgs/:orgSlug")
	org.GET("/gatewayclasses", h.withOrgPermission("gateway.read", h.listGatewayClasses))
	org.GET("/istio-profiles", h.withOrgPermission("gateway.read", h.listIstioProfiles))
	org.GET("/istio-instances", h.withOrgPermission("gateway.read", h.listIstioInstances))
	org.GET("/namespaces", h.withOrgPermission("gateway.read", h.listNamespaces))
	org.POST("/namespaces", h.withOrgPermission("gateway.write", h.createNamespace))
	org.POST("/namespaces/adopt", h.withOrgAdmin(h.adoptNamespace))

	org.GET("/oidc", h.withOrgPermission("organizations.read", h.getOIDCConfig))
	org.PUT("/oidc", h.withOrgPermission("organizations.write", h.upsertOIDCConfig))
	org.GET("/oidc/mappings", h.withOrgPermission("organizations.read", h.listOIDCMappings))
	org.POST("/oidc/mappings", h.withOrgPermission("organizations.write", h.createOIDCMapping))
	org.DELETE("/oidc/mappings/:mappingID", h.withOrgPermission("organizations.write", h.deleteOIDCMapping))

	org.GET("/rbac/users", h.withOrgPermission("rbac.read", h.listMemberships))
	org.POST("/rbac/users", h.withOrgPermission("rbac.write", h.upsertMembership))
	org.GET("/rbac/groups", h.withOrgPermission("rbac.read", h.listGroups))
	org.POST("/rbac/groups", h.withOrgPermission("rbac.write", h.createGroup))
	org.DELETE("/rbac/groups/:groupID", h.withOrgPermission("rbac.write", h.deleteGroup))
	org.GET("/rbac/groups/:groupID/users", h.withOrgPermission("rbac.read", h.listGroupMembers))
	org.POST("/rbac/groups/:groupID/users", h.withOrgPermission("rbac.write", h.addGroupMember))
	org.DELETE("/rbac/groups/:groupID/users/:userID", h.withOrgPermission("rbac.write", h.removeGroupMember))
	org.GET("/rbac/groups/:groupID/permissions", h.withOrgPermission("rbac.read", h.listGroupPermissions))
	org.POST("/rbac/groups/:groupID/permissions", h.withOrgPermission("rbac.write", h.addGroupPermission))
	org.DELETE("/rbac/groups/:groupID/permissions/:permission", h.withOrgPermission("rbac.write", h.removeGroupPermission))
	org.GET("/permissions", h.withOrgPermission("rbac.read", h.listPermissions))
	org.POST("/permissions", h.withOrgPermission("rbac.write", h.createPermission))
	org.GET("/audit-events", h.withOrgPermission("organizations.read", h.listAuditEvents))

	registerResourceRoutes(org, "gateways", kube.GatewayGVR(), "gateway.read", "gateway.write", h)
	registerResourceRoutes(org, "httproutes", kube.HTTPRouteGVR(), "gateway.read", "gateway.write", h)
	registerResourceRoutes(org, "authpolicies", kube.AuthPolicyGVR(), "security.read", "security.write", h)
	registerResourceRoutes(org, "ratelimitpolicies", kube.RateLimitPolicyGVR(), "security.read", "security.write", h)
}

// me godoc
// @Summary Current user
// @Tags auth
// @Produce json
// @Success 200 {object} models.User
// @Router /api/v1/me [get]
func (h *Handler) me(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	c.JSON(http.StatusOK, toCurrentUserResponse(user))
}

func (h *Handler) listOrganizations(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	orgs, err := h.rbac.ListOrganizationsForUser(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, toOrganizationResponses(orgs))
}

func (h *Handler) createOrganization(c *gin.Context) {
	user, _ := middleware.UserFromContext(c)
	var req CreateOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	org, err := h.rbac.CreateOrganizationWithAdmin(user, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toOrganizationResponse(org))
}

func registerResourceRoutes(rg *gin.RouterGroup, path string, gvr schema.GroupVersionResource, readPerm, writePerm string, h *Handler) {
	resource := rg.Group("/" + path)
	resource.GET("", h.withOrgPermission(readPerm, h.listResource(gvr)))
	resource.POST("", h.withOrgPermission(writePerm, h.upsertResource(gvr)))
	resource.DELETE("/:namespace/:name", h.withOrgPermission(writePerm, h.deleteResource(gvr)))
}

func (h *Handler) listResource(gvr schema.GroupVersionResource) gin.HandlerFunc {
	return func(c *gin.Context) {
		org := c.MustGet("org").(models.Organization)
		ns := strings.TrimSpace(c.Query("namespace"))
		if ns == "" {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "namespace query param is required"})
			return
		}
		if !h.ensureNamespaceOwnership(c, org.ID, ns) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		items, err := h.kubeSvc.List(ctx, gvr, ns)
		if err != nil {
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, items)
	}
}

func (h *Handler) upsertResource(gvr schema.GroupVersionResource) gin.HandlerFunc {
	return func(c *gin.Context) {
		org := c.MustGet("org").(models.Organization)
		user, _ := middleware.UserFromContext(c)
		var req UpsertResourceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		req.Namespace = strings.TrimSpace(req.Namespace)
		if req.Namespace == "" {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "namespace is required"})
			return
		}
		if fieldErrs := validation.ValidateResourceSpec(gvr.Resource, req.Spec); len(fieldErrs) > 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:       "semantic validation failed",
				FieldErrors: fieldErrs,
			})
			return
		}
		if !h.ensureNamespaceOwnership(c, org.ID, req.Namespace) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		if err := h.kubeSvc.UpsertGeneric(ctx, gvr, req.Namespace, req.Name, req.Spec); err != nil {
			_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "resource.apply.failed", gvr.Resource, "failed", "resource apply failed", map[string]any{
				"namespace": req.Namespace,
				"name":      req.Name,
				"error":     err.Error(),
			})
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
			return
		}
		_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "resource.apply", gvr.Resource, "success", "resource applied", map[string]any{
			"namespace": req.Namespace,
			"name":      req.Name,
		})
		c.JSON(http.StatusOK, statusResponse{Status: "ok"})
	}
}

func (h *Handler) deleteResource(gvr schema.GroupVersionResource) gin.HandlerFunc {
	return func(c *gin.Context) {
		org := c.MustGet("org").(models.Organization)
		user, _ := middleware.UserFromContext(c)
		ns := c.Param("namespace")
		name := c.Param("name")
		if !h.ensureNamespaceOwnership(c, org.ID, ns) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		if err := h.kubeSvc.Delete(ctx, gvr, ns, name); err != nil {
			_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "resource.delete.failed", gvr.Resource, "failed", "resource delete failed", map[string]any{
				"namespace": ns,
				"name":      name,
				"error":     err.Error(),
			})
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
			return
		}
		_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "resource.delete", gvr.Resource, "success", "resource deleted", map[string]any{
			"namespace": ns,
			"name":      name,
		})
		c.JSON(http.StatusOK, statusResponse{Status: "deleted"})
	}
}

func (h *Handler) listGatewayClasses(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	classes, err := h.kubeSvc.ListGatewayClasses(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, classes)
}

func (h *Handler) listIstioProfiles(c *gin.Context) {
	c.JSON(http.StatusOK, service.NamespaceProfiles())
}

func (h *Handler) listIstioInstances(c *gin.Context) {
	c.JSON(http.StatusOK, service.NamespaceInstances())
}

func (h *Handler) listNamespaces(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	namespaces, err := h.rbac.ListOrgNamespaces(org.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	out := make([]namespaceStatusResponse, 0, len(namespaces))
	for _, ns := range namespaces {
		exists, existsErr := h.kubeSvc.NamespaceExists(ctx, ns.Namespace)
		if existsErr != nil {
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: existsErr.Error()})
			return
		}
		out = append(out, namespaceStatusResponse{Name: ns.Namespace, Exists: exists})
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) createNamespace(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	user, _ := middleware.UserFromContext(c)
	var req CreateNamespaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	defaultInstance := ""
	defaultProfile := ""
	if len(org.Settings) > 0 {
		settings := map[string]any{}
		if err := json.Unmarshal(org.Settings, &settings); err == nil {
			if v, ok := settings["defaultNamespaceInstance"].(string); ok {
				defaultInstance = strings.TrimSpace(v)
			}
			if v, ok := settings["defaultNamespaceProfile"].(string); ok {
				defaultProfile = strings.TrimSpace(v)
			}
		}
	}
	if req.Instance == "" {
		req.Instance = defaultInstance
	}
	if req.Profile == "" {
		req.Profile = defaultProfile
	}
	if req.Profile == "" {
		req.Profile = "default"
	}
	if req.Instance == "" {
		req.Instance = "default"
	}
	extra := map[string]string{}
	for _, kv := range req.Labels {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		extra[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	if err := h.kubeSvc.CreateNamespace(ctx, req.Name, req.Instance, req.Profile, extra); err != nil {
		_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "namespace.create.failed", "namespace", "failed", "namespace creation failed", map[string]any{
			"namespace": req.Name,
			"instance":  req.Instance,
			"profile":   req.Profile,
			"error":     err.Error(),
		})
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.rbac.ClaimNamespace(org.ID, user.ID, req.Name); err != nil {
		if errors.Is(err, rbac.ErrNamespaceOwnedByAnotherOrg) {
			c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "namespace.create", "namespace", "success", "namespace created and claimed", map[string]any{
		"namespace": req.Name,
		"instance":  req.Instance,
		"profile":   req.Profile,
	})
	c.JSON(http.StatusCreated, statusResponse{Status: "created"})
}

func (h *Handler) adoptNamespace(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	user, _ := middleware.UserFromContext(c)

	var req AdoptNamespaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "namespace name is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	exists, err := h.kubeSvc.NamespaceExists(ctx, req.Name)
	if err != nil {
		_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "namespace.adopt.failed", "namespace", "failed", "namespace adoption lookup failed", map[string]any{
			"namespace": req.Name,
			"error":     err.Error(),
		})
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "namespace not found in cluster"})
		return
	}

	if err := h.rbac.ClaimNamespace(org.ID, user.ID, req.Name); err != nil {
		if errors.Is(err, rbac.ErrNamespaceOwnedByAnotherOrg) {
			c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	_ = h.rbac.RecordAuditEvent(org.ID, user.ID, "namespace.adopt", "namespace", "success", "existing namespace adopted", map[string]any{
		"namespace": req.Name,
	})
	c.JSON(http.StatusCreated, statusResponse{Status: "adopted"})
}

func (h *Handler) getOIDCConfig(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	cfg, err := h.rbac.GetOIDCConfig(org.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, toOIDCConfigResponse(cfg))
}

func (h *Handler) upsertOIDCConfig(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	var req UpsertOIDCConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	cfg := models.OIDCConfig{
		IssuerURL:     req.IssuerURL,
		ClientID:      req.ClientID,
		ClientSecret:  req.ClientSecret,
		GroupClaim:    req.GroupClaim,
		UsernameClaim: req.UsernameClaim,
		Enabled:       req.Enabled,
	}
	if cfg.GroupClaim == "" {
		cfg.GroupClaim = "groups"
	}
	if cfg.UsernameClaim == "" {
		cfg.UsernameClaim = "email"
	}
	if err := h.rbac.UpsertOIDCConfig(org.ID, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, statusResponse{Status: "ok"})
}

func (h *Handler) listOIDCMappings(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	mappings, err := h.rbac.ListOIDCMapping(org.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, toOIDCMappingResponses(mappings))
}

func (h *Handler) createOIDCMapping(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	var req CreateOIDCMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	externalValue := strings.TrimSpace(req.ExternalValue)
	externalGroup := strings.TrimSpace(req.ExternalGroup)
	if externalValue == "" {
		externalValue = externalGroup
	}
	if externalValue == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "externalValue is required"})
		return
	}
	mapping := models.OIDCMapping{
		SubjectType:      strings.TrimSpace(req.SubjectType),
		ExternalValue:    externalValue,
		ExternalGroup:    externalGroup,
		MappedRole:       strings.TrimSpace(req.MappedRole),
		CustomPermission: strings.TrimSpace(req.CustomPermission),
	}
	if err := h.rbac.CreateOIDCMapping(org.ID, mapping); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "required") {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, statusResponse{Status: "created"})
}

func (h *Handler) deleteOIDCMapping(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	id, err := uuid.Parse(c.Param("mappingID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid mapping id"})
		return
	}
	if err := h.rbac.DeleteOIDCMapping(org.ID, id); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, statusResponse{Status: "deleted"})
}

func (h *Handler) listMemberships(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	memberships, err := h.rbac.ListMemberships(org.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, toMembershipResponses(memberships))
}

func (h *Handler) upsertMembership(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	var req UpsertMembershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.rbac.UpsertMembership(org.ID, req.Email, req.Role, req.CustomPermissions); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, statusResponse{Status: "ok"})
}

func (h *Handler) listGroups(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	groups, err := h.rbac.ListGroups(org.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, toGroupResponses(groups))
}

func (h *Handler) createGroup(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	group, err := h.rbac.CreateGroup(org.ID, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toGroupResponse(group))
}

func (h *Handler) deleteGroup(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid group id"})
		return
	}
	if err := h.rbac.DeleteGroup(org.ID, groupID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, statusResponse{Status: "deleted"})
}

func (h *Handler) listGroupMembers(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid group id"})
		return
	}
	users, err := h.rbac.ListGroupMembers(org.ID, groupID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, toGroupMemberResponses(users))
}

func (h *Handler) addGroupMember(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid group id"})
		return
	}
	var req AddGroupMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	user, err := h.rbac.AddGroupMemberByEmail(org.ID, groupID, req.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "group not found"})
			return
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, groupMemberResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
	})
}

func (h *Handler) removeGroupMember(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid group id"})
		return
	}
	userID, err := uuid.Parse(c.Param("userID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid user id"})
		return
	}
	if err := h.rbac.RemoveGroupMember(org.ID, groupID, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, statusResponse{Status: "deleted"})
}

func (h *Handler) listGroupPermissions(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid group id"})
		return
	}
	perms, err := h.rbac.ListGroupPermissions(org.ID, groupID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, toGroupPermissionResponses(perms))
}

func (h *Handler) addGroupPermission(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid group id"})
		return
	}
	var req AddGroupPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.rbac.AddGroupPermission(org.ID, groupID, req.Permission); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "group not found"})
			return
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, groupPermissionResponse{Permission: strings.TrimSpace(req.Permission)})
}

func (h *Handler) removeGroupPermission(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	groupID, err := uuid.Parse(c.Param("groupID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid group id"})
		return
	}
	permission := strings.TrimSpace(c.Param("permission"))
	if permission == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "permission is required"})
		return
	}
	if err := h.rbac.RemoveGroupPermission(org.ID, groupID, permission); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, statusResponse{Status: "deleted"})
}

func (h *Handler) listPermissions(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	permissions, err := h.rbac.ListPermissions(org.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, toPermissionResponses(permissions))
}

func (h *Handler) createPermission(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	var req CreatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.rbac.CreatePermission(org.ID, models.Permission{Name: req.Name, Resource: req.Resource, Action: req.Action, Definition: req.Definition}); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, statusResponse{Status: "created"})
}

func (h *Handler) listAuditEvents(c *gin.Context) {
	org := c.MustGet("org").(models.Organization)
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	resource := strings.TrimSpace(c.Query("resource"))
	status := strings.TrimSpace(c.Query("status"))
	eventType := strings.TrimSpace(c.Query("eventType"))
	events, err := h.rbac.ListAuditEventsFiltered(org.ID, limit, resource, status, eventType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, toAuditEventResponses(events))
}

func (h *Handler) withOrgPermission(permission string, next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := middleware.UserFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "authentication required"})
			return
		}
		orgSlug := c.Param("orgSlug")
		org, err := h.rbac.Authorize(user.ID, orgSlug, permission)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, ErrorResponse{Error: "organization not found or no membership"})
				return
			}
			c.JSON(http.StatusForbidden, ErrorResponse{Error: err.Error()})
			return
		}
		c.Set("org", org)
		next(c)
	}
}

func (h *Handler) withOrgAdmin(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := middleware.UserFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "authentication required"})
			return
		}
		orgSlug := c.Param("orgSlug")
		org, isAdmin, err := h.rbac.IsOrgAdmin(user.ID, orgSlug)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, ErrorResponse{Error: "organization not found or no membership"})
				return
			}
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
		if !isAdmin {
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "admin role required"})
			return
		}
		c.Set("org", org)
		next(c)
	}
}

func (h *Handler) ensureNamespaceOwnership(c *gin.Context, orgID uuid.UUID, namespace string) bool {
	ok, err := h.rbac.NamespaceBelongsToOrg(orgID, namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed namespace ownership check"})
		return false
	}
	if !ok {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "namespace is not owned by this organization"})
		return false
	}
	return true
}
