package api

import (
	"encoding/json"
	"time"

	"github.com/egevorky/qdash/internal/models"
	"github.com/google/uuid"
)

type statusResponse struct {
	Status string `json:"status"`
}

type currentUserResponse struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	Source      string    `json:"source"`
}

type organizationResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

type oidcConfigResponse struct {
	ID            uuid.UUID `json:"id"`
	IssuerURL     string    `json:"issuerUrl"`
	ClientID      string    `json:"clientId"`
	GroupClaim    string    `json:"groupClaim"`
	UsernameClaim string    `json:"usernameClaim"`
	Enabled       bool      `json:"enabled"`
}

type oidcMappingResponse struct {
	ID               uuid.UUID `json:"id"`
	SubjectType      string    `json:"subjectType"`
	ExternalValue    string    `json:"externalValue"`
	ExternalGroup    string    `json:"externalGroup"`
	MappedRole       string    `json:"mappedRole"`
	CustomPermission string    `json:"customPermission"`
}

type membershipResponse struct {
	ID         uuid.UUID `json:"id"`
	Role       string    `json:"role"`
	Permission string    `json:"permission"`
}

type permissionResponse struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Resource   string    `json:"resource"`
	Action     string    `json:"action"`
	Definition string    `json:"definition"`
	IsBuiltIn  bool      `json:"isBuiltIn"`
}

type groupResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type groupMemberResponse struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
}

type groupPermissionResponse struct {
	Permission string `json:"permission"`
}

type namespaceStatusResponse struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
}

type auditEventResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	EventType string    `json:"eventType"`
	Resource  string    `json:"resource"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	ActorID   uuid.UUID `json:"actorId"`
	Details   any       `json:"details"`
}

func toCurrentUserResponse(user models.User) currentUserResponse {
	return currentUserResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Source:      user.Source,
	}
}

func toOrganizationResponse(org models.Organization) organizationResponse {
	return organizationResponse{
		ID:   org.ID,
		Name: org.Name,
		Slug: org.Slug,
	}
}

func toOrganizationResponses(orgs []models.Organization) []organizationResponse {
	out := make([]organizationResponse, 0, len(orgs))
	for _, org := range orgs {
		out = append(out, toOrganizationResponse(org))
	}
	return out
}

func toOIDCConfigResponse(cfg models.OIDCConfig) oidcConfigResponse {
	return oidcConfigResponse{
		ID:            cfg.ID,
		IssuerURL:     cfg.IssuerURL,
		ClientID:      cfg.ClientID,
		GroupClaim:    cfg.GroupClaim,
		UsernameClaim: cfg.UsernameClaim,
		Enabled:       cfg.Enabled,
	}
}

func toOIDCMappingResponses(mappings []models.OIDCMapping) []oidcMappingResponse {
	out := make([]oidcMappingResponse, 0, len(mappings))
	for _, m := range mappings {
		subjectType := m.SubjectType
		if subjectType == "" {
			subjectType = "group"
		}
		externalValue := m.ExternalValue
		if externalValue == "" {
			externalValue = m.ExternalGroup
		}
		out = append(out, oidcMappingResponse{
			ID:               m.ID,
			SubjectType:      subjectType,
			ExternalValue:    externalValue,
			ExternalGroup:    m.ExternalGroup,
			MappedRole:       m.MappedRole,
			CustomPermission: m.CustomPermission,
		})
	}
	return out
}

func toMembershipResponses(memberships []models.Membership) []membershipResponse {
	out := make([]membershipResponse, 0, len(memberships))
	for _, m := range memberships {
		out = append(out, membershipResponse{
			ID:         m.ID,
			Role:       m.Role,
			Permission: m.Permission,
		})
	}
	return out
}

func toPermissionResponses(permissions []models.Permission) []permissionResponse {
	out := make([]permissionResponse, 0, len(permissions))
	for _, p := range permissions {
		out = append(out, permissionResponse{
			ID:         p.ID,
			Name:       p.Name,
			Resource:   p.Resource,
			Action:     p.Action,
			Definition: p.Definition,
			IsBuiltIn:  p.IsBuiltIn,
		})
	}
	return out
}

func toGroupResponse(group models.Group) groupResponse {
	return groupResponse{ID: group.ID, Name: group.Name}
}

func toGroupResponses(groups []models.Group) []groupResponse {
	out := make([]groupResponse, 0, len(groups))
	for _, g := range groups {
		out = append(out, toGroupResponse(g))
	}
	return out
}

func toGroupMemberResponses(users []models.User) []groupMemberResponse {
	out := make([]groupMemberResponse, 0, len(users))
	for _, u := range users {
		out = append(out, groupMemberResponse{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
		})
	}
	return out
}

func toGroupPermissionResponses(perms []models.GroupPermission) []groupPermissionResponse {
	out := make([]groupPermissionResponse, 0, len(perms))
	for _, p := range perms {
		out = append(out, groupPermissionResponse{Permission: p.Permission})
	}
	return out
}

func toAuditEventResponses(events []models.AuditEvent) []auditEventResponse {
	out := make([]auditEventResponse, 0, len(events))
	for _, e := range events {
		var details any = map[string]any{}
		if len(e.Details) > 0 {
			_ = json.Unmarshal(e.Details, &details)
		}
		out = append(out, auditEventResponse{
			ID:        e.ID,
			CreatedAt: e.CreatedAt,
			EventType: e.EventType,
			Resource:  e.Resource,
			Status:    e.Status,
			Message:   e.Message,
			ActorID:   e.ActorUserID,
			Details:   details,
		})
	}
	return out
}
