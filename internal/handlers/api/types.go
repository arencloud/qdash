package api

import "github.com/egevorky/qdash/internal/validation"

type ErrorResponse struct {
	Error       string                  `json:"error"`
	FieldErrors []validation.FieldError `json:"fieldErrors,omitempty"`
}

type CreateOrganizationRequest struct {
	Name string `json:"name" binding:"required,min=3,max=120"`
}

type CreateNamespaceRequest struct {
	Name     string   `json:"name" binding:"required"`
	Instance string   `json:"instance"`
	Profile  string   `json:"profile"`
	Labels   []string `json:"labels"`
}

type AdoptNamespaceRequest struct {
	Name string `json:"name" binding:"required"`
}

type UpsertResourceRequest struct {
	Namespace string         `json:"namespace" binding:"required"`
	Name      string         `json:"name" binding:"required"`
	Spec      map[string]any `json:"spec" binding:"required"`
}

type UpsertOIDCConfigRequest struct {
	IssuerURL     string `json:"issuerUrl" binding:"required"`
	ClientID      string `json:"clientId" binding:"required"`
	ClientSecret  string `json:"clientSecret" binding:"required"`
	GroupClaim    string `json:"groupClaim"`
	UsernameClaim string `json:"usernameClaim"`
	Enabled       bool   `json:"enabled"`
}

type CreateOIDCMappingRequest struct {
	SubjectType      string `json:"subjectType"`
	ExternalValue    string `json:"externalValue"`
	ExternalGroup    string `json:"externalGroup"`
	MappedRole       string `json:"mappedRole" binding:"required"`
	CustomPermission string `json:"customPermission"`
}

type UpsertMembershipRequest struct {
	Email             string   `json:"email" binding:"required,email"`
	Role              string   `json:"role" binding:"required"`
	CustomPermissions []string `json:"customPermissions"`
}

type CreatePermissionRequest struct {
	Name       string `json:"name" binding:"required"`
	Resource   string `json:"resource" binding:"required"`
	Action     string `json:"action" binding:"required"`
	Definition string `json:"definition"`
}

type CreateGroupRequest struct {
	Name string `json:"name" binding:"required,min=2,max=120"`
}

type AddGroupMemberRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type AddGroupPermissionRequest struct {
	Permission string `json:"permission" binding:"required"`
}
