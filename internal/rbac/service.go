package rbac

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/arencloud/qdash/internal/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var slugRe = regexp.MustCompile(`[^a-z0-9-]`)
var ErrNamespaceOwnedByAnotherOrg = errors.New("namespace already owned by another organization")

var DefaultPermissions = []models.Permission{
	{Name: "organizations.read", Resource: "organizations", Action: "read", IsBuiltIn: true},
	{Name: "organizations.write", Resource: "organizations", Action: "write", IsBuiltIn: true},
	{Name: "rbac.read", Resource: "rbac", Action: "read", IsBuiltIn: true},
	{Name: "rbac.write", Resource: "rbac", Action: "write", IsBuiltIn: true},
	{Name: "security.read", Resource: "policies", Action: "read", IsBuiltIn: true},
	{Name: "gateway.read", Resource: "gateway", Action: "read", IsBuiltIn: true},
	{Name: "gateway.write", Resource: "gateway", Action: "write", IsBuiltIn: true},
	{Name: "security.write", Resource: "policies", Action: "write", IsBuiltIn: true},
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) CreateOrganizationWithAdmin(user models.User, name string) (models.Organization, error) {
	slug := slugify(name)
	if slug == "" {
		return models.Organization{}, errors.New("organization name is invalid")
	}
	org := models.Organization{Name: name, Slug: slug}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&org).Error; err != nil {
			return err
		}

		for _, p := range DefaultPermissions {
			perm := p
			perm.OrgID = org.ID
			if err := tx.Create(&perm).Error; err != nil {
				return err
			}
		}

		membership := models.Membership{OrgID: org.ID, UserID: user.ID, Role: "admin"}
		if err := tx.Create(&membership).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return models.Organization{}, fmt.Errorf("create org: %w", err)
	}
	return org, nil
}

func (s *Service) ListOrganizationsForUser(userID uuid.UUID) ([]models.Organization, error) {
	var orgs []models.Organization
	err := s.db.Table("organizations").
		Select("organizations.*").
		Joins("join memberships on memberships.org_id = organizations.id").
		Where("memberships.user_id = ?", userID).
		Order("organizations.created_at desc").
		Find(&orgs).Error
	return orgs, err
}

func (s *Service) UpdateOrganization(orgID uuid.UUID, name, description string) error {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return errors.New("organization name is required")
	}
	return s.db.Model(&models.Organization{}).
		Where("id = ?", orgID).
		Updates(map[string]any{
			"name":        name,
			"description": description,
		}).Error
}

func (s *Service) UpdateOrganizationSettings(orgID uuid.UUID, settings map[string]any) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return s.db.Model(&models.Organization{}).
		Where("id = ?", orgID).
		Update("settings", datatypes.JSON(data)).Error
}

func (s *Service) ResolveOrgForUser(userID uuid.UUID, slug string) (models.Organization, models.Membership, error) {
	var org models.Organization
	if err := s.db.Where("slug = ?", slug).First(&org).Error; err != nil {
		return models.Organization{}, models.Membership{}, err
	}
	var membership models.Membership
	err := s.db.Where("org_id = ? AND user_id = ?", org.ID, userID).First(&membership).Error
	if err != nil {
		return models.Organization{}, models.Membership{}, err
	}
	return org, membership, nil
}

func (s *Service) Authorize(userID uuid.UUID, slug, permission string) (models.Organization, error) {
	org, membership, err := s.ResolveOrgForUser(userID, slug)
	if err != nil {
		return models.Organization{}, err
	}
	groupPerms, err := s.userGroupPermissionSet(org.ID, userID)
	if err != nil {
		return models.Organization{}, err
	}
	if hasPermission(membership, groupPerms, permission) {
		return org, nil
	}
	return models.Organization{}, fmt.Errorf("forbidden: missing permission %s", permission)
}

func (s *Service) IsOrgAdmin(userID uuid.UUID, slug string) (models.Organization, bool, error) {
	org, membership, err := s.ResolveOrgForUser(userID, slug)
	if err != nil {
		return models.Organization{}, false, err
	}
	return org, membership.Role == "admin", nil
}

func (s *Service) ListMemberships(orgID uuid.UUID) ([]models.Membership, error) {
	var memberships []models.Membership
	err := s.db.Where("org_id = ?", orgID).Order("created_at desc").Find(&memberships).Error
	return memberships, err
}

func (s *Service) ListGroups(orgID uuid.UUID) ([]models.Group, error) {
	var groups []models.Group
	err := s.db.Where("org_id = ?", orgID).Order("name asc").Find(&groups).Error
	return groups, err
}

func (s *Service) CreateGroup(orgID uuid.UUID, name string) (models.Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.Group{}, errors.New("group name is required")
	}
	g := models.Group{OrgID: orgID, Name: name}
	if err := s.db.Create(&g).Error; err != nil {
		return models.Group{}, err
	}
	return g, nil
}

func (s *Service) DeleteGroup(orgID, groupID uuid.UUID) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var group models.Group
		if err := tx.Where("id = ? AND org_id = ?", groupID, orgID).First(&group).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ? AND group_id = ?", orgID, groupID).Delete(&models.GroupMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ? AND group_id = ?", orgID, groupID).Delete(&models.GroupPermission{}).Error; err != nil {
			return err
		}
		return tx.Delete(&group).Error
	})
}

func (s *Service) ListGroupMembers(orgID, groupID uuid.UUID) ([]models.User, error) {
	if _, err := s.ensureGroupInOrg(orgID, groupID); err != nil {
		return nil, err
	}
	var users []models.User
	err := s.db.Table("users").
		Select("users.*").
		Joins("join group_members on group_members.user_id = users.id").
		Where("group_members.org_id = ? AND group_members.group_id = ?", orgID, groupID).
		Order("users.email asc").
		Find(&users).Error
	return users, err
}

func (s *Service) AddGroupMemberByEmail(orgID, groupID uuid.UUID, email string) (models.User, error) {
	if _, err := s.ensureGroupInOrg(orgID, groupID); err != nil {
		return models.User{}, err
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return models.User{}, errors.New("email is required")
	}
	var user models.User
	err := s.db.Where("email = ?", email).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		user = models.User{Email: email, DisplayName: strings.Split(email, "@")[0], Source: "local"}
		if err := s.db.Create(&user).Error; err != nil {
			return models.User{}, err
		}
	} else if err != nil {
		return models.User{}, err
	}
	member := models.GroupMember{OrgID: orgID, GroupID: groupID, UserID: user.ID}
	if err := s.db.Where("org_id = ? AND group_id = ? AND user_id = ?", orgID, groupID, user.ID).First(&models.GroupMember{}).Error; err == nil {
		return user, nil
	} else if err != gorm.ErrRecordNotFound {
		return models.User{}, err
	}
	return user, s.db.Create(&member).Error
}

func (s *Service) RemoveGroupMember(orgID, groupID, userID uuid.UUID) error {
	if _, err := s.ensureGroupInOrg(orgID, groupID); err != nil {
		return err
	}
	return s.db.Where("org_id = ? AND group_id = ? AND user_id = ?", orgID, groupID, userID).Delete(&models.GroupMember{}).Error
}

func (s *Service) ListGroupPermissions(orgID, groupID uuid.UUID) ([]models.GroupPermission, error) {
	if _, err := s.ensureGroupInOrg(orgID, groupID); err != nil {
		return nil, err
	}
	var out []models.GroupPermission
	err := s.db.Where("org_id = ? AND group_id = ?", orgID, groupID).Order("permission asc").Find(&out).Error
	return out, err
}

func (s *Service) AddGroupPermission(orgID, groupID uuid.UUID, permission string) error {
	if _, err := s.ensureGroupInOrg(orgID, groupID); err != nil {
		return err
	}
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return errors.New("permission is required")
	}
	var exists int64
	if err := s.db.Model(&models.Permission{}).
		Where("org_id = ? AND name = ?", orgID, permission).
		Count(&exists).Error; err != nil {
		return err
	}
	if exists == 0 {
		return errors.New("permission does not exist in organization")
	}
	var gp models.GroupPermission
	err := s.db.Where("org_id = ? AND group_id = ? AND permission = ?", orgID, groupID, permission).First(&gp).Error
	if err == nil {
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return s.db.Create(&models.GroupPermission{OrgID: orgID, GroupID: groupID, Permission: permission}).Error
}

func (s *Service) RemoveGroupPermission(orgID, groupID uuid.UUID, permission string) error {
	if _, err := s.ensureGroupInOrg(orgID, groupID); err != nil {
		return err
	}
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return errors.New("permission is required")
	}
	return s.db.Where("org_id = ? AND group_id = ? AND permission = ?", orgID, groupID, permission).Delete(&models.GroupPermission{}).Error
}

func (s *Service) UpsertMembership(orgID uuid.UUID, email, role string, customPermissions []string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return errors.New("email is required")
	}
	var user models.User
	err := s.db.Where("email = ?", email).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		user = models.User{Email: email, DisplayName: strings.Split(email, "@")[0], Source: "local"}
		if err := s.db.Create(&user).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	permCSV := joinPermissions(customPermissions)
	var membership models.Membership
	err = s.db.Where("org_id = ? AND user_id = ?", orgID, user.ID).First(&membership).Error
	if err == gorm.ErrRecordNotFound {
		return s.db.Create(&models.Membership{
			OrgID:      orgID,
			UserID:     user.ID,
			Role:       role,
			Permission: permCSV,
		}).Error
	}
	if err != nil {
		return err
	}
	membership.Role = role
	membership.Permission = permCSV
	return s.db.Save(&membership).Error
}

func (s *Service) GetOIDCConfig(orgID uuid.UUID) (models.OIDCConfig, error) {
	var cfg models.OIDCConfig
	err := s.db.Where("org_id = ?", orgID).First(&cfg).Error
	return cfg, err
}

func (s *Service) UpsertOIDCConfig(orgID uuid.UUID, cfg models.OIDCConfig) error {
	var existing models.OIDCConfig
	err := s.db.Where("org_id = ?", orgID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		cfg.OrgID = orgID
		return s.db.Create(&cfg).Error
	}
	if err != nil {
		return err
	}
	existing.IssuerURL = cfg.IssuerURL
	existing.ClientID = cfg.ClientID
	existing.ClientSecret = cfg.ClientSecret
	existing.GroupClaim = cfg.GroupClaim
	existing.UsernameClaim = cfg.UsernameClaim
	existing.Enabled = cfg.Enabled
	return s.db.Save(&existing).Error
}

func (s *Service) ListOIDCMapping(orgID uuid.UUID) ([]models.OIDCMapping, error) {
	var mappings []models.OIDCMapping
	err := s.db.Where("org_id = ?", orgID).Order("created_at desc").Find(&mappings).Error
	return mappings, err
}

func (s *Service) CreateOIDCMapping(orgID uuid.UUID, mapping models.OIDCMapping) error {
	mapping.OrgID = orgID
	mapping.SubjectType = normalizeOIDCSubjectType(mapping.SubjectType)
	mapping.ExternalValue = strings.TrimSpace(mapping.ExternalValue)
	mapping.ExternalGroup = strings.TrimSpace(mapping.ExternalGroup)
	if mapping.ExternalValue == "" {
		mapping.ExternalValue = mapping.ExternalGroup
	}
	if mapping.ExternalGroup == "" {
		mapping.ExternalGroup = mapping.ExternalValue
	}
	if mapping.ExternalValue == "" {
		return errors.New("external value is required")
	}
	mapping.MappedRole = normalizeRole(mapping.MappedRole)
	if mapping.MappedRole == "" {
		return errors.New("mapped role is required")
	}
	return s.db.Create(&mapping).Error
}

func (s *Service) DeleteOIDCMapping(orgID, mappingID uuid.UUID) error {
	return s.db.Where("id = ? AND org_id = ?", mappingID, orgID).Delete(&models.OIDCMapping{}).Error
}

func (s *Service) ListPermissions(orgID uuid.UUID) ([]models.Permission, error) {
	var out []models.Permission
	err := s.db.Where("org_id = ?", orgID).Order("name asc").Find(&out).Error
	return out, err
}

func (s *Service) CreatePermission(orgID uuid.UUID, permission models.Permission) error {
	permission.OrgID = orgID
	return s.db.Create(&permission).Error
}

func (s *Service) ApplyOIDCMappings(userID uuid.UUID, rawClaims map[string]any) error {
	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	var configs []models.OIDCConfig
	if err := s.db.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		return err
	}
	for _, cfg := range configs {
		groups := claimStrings(rawClaims, cfg.GroupClaim)
		groupSet := toLowerSet(groups)
		userSet := toLowerSet(claimStrings(rawClaims, cfg.UsernameClaim))
		if user.Email != "" {
			userSet[strings.ToLower(strings.TrimSpace(user.Email))] = true
		}
		roleSet := toLowerSet(claimStrings(rawClaims, "roles"))
		if len(roleSet) == 0 {
			roleSet = toLowerSet(claimStrings(rawClaims, "role"))
		}
		for _, role := range nestedClaimStrings(rawClaims, "realm_access", "roles") {
			roleSet[strings.ToLower(strings.TrimSpace(role))] = true
		}
		for _, role := range nestedClaimStrings(rawClaims, "resource_access", cfg.ClientID, "roles") {
			roleSet[strings.ToLower(strings.TrimSpace(role))] = true
		}

		var mappings []models.OIDCMapping
		if err := s.db.Where("org_id = ?", cfg.OrgID).Find(&mappings).Error; err != nil {
			return err
		}
		if len(mappings) == 0 {
			_ = s.logAuditEvent(cfg.OrgID, userID, "oidc.mapping.skipped", "oidc_mapping", "skipped", "no OIDC mappings configured for organization", map[string]any{
				"groupClaim": cfg.GroupClaim,
				"groups":     groups,
			})
			continue
		}

		role := ""
		customPerms := []string{}
		matchedSubjects := []string{}
		for _, m := range mappings {
			subjectType := normalizeOIDCSubjectType(m.SubjectType)
			externalValue := strings.ToLower(strings.TrimSpace(m.ExternalValue))
			if externalValue == "" {
				externalValue = strings.ToLower(strings.TrimSpace(m.ExternalGroup))
			}
			if externalValue == "" || !oidcSubjectMatched(subjectType, externalValue, groupSet, userSet, roleSet) {
				continue
			}
			matchedSubjects = append(matchedSubjects, subjectType+":"+externalValue)
			role = strongerRole(role, m.MappedRole)
			if p := strings.TrimSpace(m.CustomPermission); p != "" {
				customPerms = append(customPerms, p)
			}
		}
		if role == "" {
			_ = s.logAuditEvent(cfg.OrgID, userID, "oidc.mapping.skipped", "oidc_mapping", "skipped", "no OIDC mappings matched for organization", map[string]any{
				"groupClaim": cfg.GroupClaim,
				"userClaim":  cfg.UsernameClaim,
				"groups":     groups,
			})
			continue
		}
		if err := s.upsertMembershipForUser(cfg.OrgID, userID, role, customPerms); err != nil {
			_ = s.logAuditEvent(cfg.OrgID, userID, "oidc.mapping.failed", "membership", "failed", "failed to apply OIDC mapping to membership", map[string]any{
				"matchedSubjects":   matchedSubjects,
				"resolvedRole":      role,
				"customPermissions": customPerms,
				"error":             err.Error(),
			})
			return err
		}
		_ = s.logAuditEvent(cfg.OrgID, userID, "oidc.mapping.applied", "membership", "success", "OIDC mapping applied to membership", map[string]any{
			"matchedSubjects":   matchedSubjects,
			"resolvedRole":      role,
			"customPermissions": mergePermissions(customPerms),
			"groupClaim":        cfg.GroupClaim,
			"userClaim":         cfg.UsernameClaim,
		})
	}
	return nil
}

func (s *Service) ListAuditEvents(orgID uuid.UUID, limit int) ([]models.AuditEvent, error) {
	return s.ListAuditEventsFiltered(orgID, limit, "", "", "")
}

func (s *Service) ListAuditEventsFiltered(orgID uuid.UUID, limit int, resource, status, eventType string) ([]models.AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []models.AuditEvent
	query := s.db.Where("org_id = ?", orgID)
	if strings.TrimSpace(resource) != "" {
		query = query.Where("resource = ?", strings.TrimSpace(resource))
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(status))
	}
	if strings.TrimSpace(eventType) != "" {
		query = query.Where("event_type = ?", strings.TrimSpace(eventType))
	}
	err := query.Order("created_at desc").Limit(limit).Find(&out).Error
	return out, err
}

func (s *Service) ClaimNamespace(orgID, actorUserID uuid.UUID, namespace string) error {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return errors.New("namespace is required")
	}
	var binding models.OrgNamespace
	err := s.db.Where("namespace = ?", namespace).First(&binding).Error
	if err == nil {
		if binding.OrgID == orgID {
			return nil
		}
		return ErrNamespaceOwnedByAnotherOrg
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return s.db.Create(&models.OrgNamespace{
		OrgID:     orgID,
		Namespace: namespace,
		Cluster:   "default",
		CreatedBy: actorUserID,
	}).Error
}

func (s *Service) NamespaceBelongsToOrg(orgID uuid.UUID, namespace string) (bool, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return false, nil
	}
	var count int64
	err := s.db.Model(&models.OrgNamespace{}).
		Where("org_id = ? AND namespace = ?", orgID, namespace).
		Count(&count).Error
	return count > 0, err
}

func (s *Service) ListOrgNamespaces(orgID uuid.UUID) ([]models.OrgNamespace, error) {
	var out []models.OrgNamespace
	err := s.db.Where("org_id = ?", orgID).Order("namespace asc").Find(&out).Error
	return out, err
}

func (s *Service) GetUsersByIDs(ids []uuid.UUID) (map[uuid.UUID]models.User, error) {
	out := make(map[uuid.UUID]models.User)
	if len(ids) == 0 {
		return out, nil
	}
	unique := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return out, nil
	}
	var users []models.User
	if err := s.db.Where("id IN ?", unique).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		out[u.ID] = u
	}
	return out, nil
}

func hasPermission(m models.Membership, groupPerms map[string]bool, permission string) bool {
	switch m.Role {
	case "admin":
		return true
	case "editor":
		if editorPermissions()[permission] {
			return true
		}
	case "viewer":
		if viewerPermissions()[permission] {
			return true
		}
	}
	for _, p := range strings.Split(m.Permission, ",") {
		if strings.TrimSpace(p) == permission {
			return true
		}
	}
	if groupPerms[permission] {
		return true
	}
	return false
}

func (s *Service) userGroupPermissionSet(orgID, userID uuid.UUID) (map[string]bool, error) {
	out := map[string]bool{}
	var rows []struct {
		Permission string
	}
	err := s.db.Table("group_permissions").
		Select("group_permissions.permission").
		Joins("join group_members on group_members.group_id = group_permissions.group_id AND group_members.org_id = group_permissions.org_id").
		Where("group_permissions.org_id = ? AND group_members.user_id = ?", orgID, userID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		p := strings.TrimSpace(row.Permission)
		if p != "" {
			out[p] = true
		}
	}
	return out, nil
}

func (s *Service) ensureGroupInOrg(orgID, groupID uuid.UUID) (models.Group, error) {
	var group models.Group
	err := s.db.Where("id = ? AND org_id = ?", groupID, orgID).First(&group).Error
	if err != nil {
		return models.Group{}, err
	}
	return group, nil
}

func (s *Service) upsertMembershipForUser(orgID, userID uuid.UUID, role string, customPermissions []string) error {
	var membership models.Membership
	err := s.db.Where("org_id = ? AND user_id = ?", orgID, userID).First(&membership).Error
	if err == gorm.ErrRecordNotFound {
		return s.db.Create(&models.Membership{
			OrgID:      orgID,
			UserID:     userID,
			Role:       normalizeRole(role),
			Permission: joinPermissions(customPermissions),
		}).Error
	}
	if err != nil {
		return err
	}

	membership.Role = strongerRole(membership.Role, role)
	membership.Permission = joinPermissions(mergePermissions(strings.Split(membership.Permission, ","), customPermissions))
	return s.db.Save(&membership).Error
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin", "editor", "viewer":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return "viewer"
	}
}

func strongerRole(a, b string) string {
	weight := map[string]int{"viewer": 1, "editor": 2, "admin": 3}
	a = normalizeRole(a)
	b = normalizeRole(b)
	if weight[b] > weight[a] {
		return b
	}
	return a
}

func claimStrings(rawClaims map[string]any, claim string) []string {
	claim = strings.TrimSpace(claim)
	if claim == "" {
		claim = "groups"
	}
	v, ok := rawClaims[claim]
	if !ok {
		return nil
	}
	switch vv := v.(type) {
	case string:
		if strings.TrimSpace(vv) == "" {
			return nil
		}
		return []string{vv}
	case []string:
		return vv
	case []any:
		out := make([]string, 0, len(vv))
		for _, it := range vv {
			s, ok := it.(string)
			if ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func nestedClaimStrings(rawClaims map[string]any, path ...string) []string {
	if len(path) == 0 {
		return nil
	}
	var current any = rawClaims
	for _, key := range path {
		if strings.TrimSpace(key) == "" {
			return nil
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[key]
		if !ok {
			return nil
		}
	}
	switch vv := current.(type) {
	case string:
		if strings.TrimSpace(vv) == "" {
			return nil
		}
		return []string{vv}
	case []string:
		return vv
	case []any:
		out := make([]string, 0, len(vv))
		for _, v := range vv {
			s, ok := v.(string)
			if ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func toLowerSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			out[v] = true
		}
	}
	return out
}

func normalizeOIDCSubjectType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "group", "user", "role":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "group"
	}
}

func oidcSubjectMatched(subjectType, externalValue string, groupSet, userSet, roleSet map[string]bool) bool {
	switch normalizeOIDCSubjectType(subjectType) {
	case "user":
		return userSet[externalValue]
	case "role":
		return roleSet[externalValue]
	default:
		return groupSet[externalValue]
	}
}

func joinPermissions(perms []string) string {
	return strings.Join(mergePermissions(perms), ",")
}

func (s *Service) RecordAuditEvent(orgID, actorUserID uuid.UUID, eventType, resource, status, message string, details map[string]any) error {
	return s.logAuditEvent(orgID, actorUserID, eventType, resource, status, message, details)
}

func (s *Service) logAuditEvent(orgID, actorUserID uuid.UUID, eventType, resource, status, message string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	event := models.AuditEvent{
		OrgID:       orgID,
		ActorUserID: actorUserID,
		EventType:   eventType,
		Resource:    resource,
		Status:      status,
		Message:     message,
		Details:     datatypes.JSON(payload),
	}
	return s.db.Create(&event).Error
}

func mergePermissions(values ...[]string) []string {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, set := range values {
		for _, v := range set {
			v = strings.TrimSpace(v)
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func editorPermissions() map[string]bool {
	return map[string]bool{
		"organizations.read": true,
		"rbac.read":          true,
		"gateway.read":       true,
		"gateway.write":      true,
		"security.read":      true,
		"security.write":     true,
	}
}

func viewerPermissions() map[string]bool {
	return map[string]bool{
		"organizations.read": true,
		"rbac.read":          true,
		"gateway.read":       true,
		"security.read":      true,
	}
}

func slugify(name string) string {
	v := strings.ToLower(strings.TrimSpace(name))
	v = strings.ReplaceAll(v, " ", "-")
	v = slugRe.ReplaceAllString(v, "")
	v = strings.Trim(v, "-")
	return v
}
