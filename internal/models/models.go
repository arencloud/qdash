package models

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Base struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey"`
	CreatedAt time.Time      `gorm:"not null"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (b *Base) BeforeCreate(_ *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

type User struct {
	Base
	Email       string `gorm:"uniqueIndex;size:320;not null"`
	DisplayName string `gorm:"size:120;not null"`
	Source      string `gorm:"size:32;not null;default:local"`
}

type AuthSession struct {
	Base
	UserID    uuid.UUID `gorm:"type:uuid;index;not null"`
	TokenHash string    `gorm:"size:64;uniqueIndex;not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
}

type OIDCAuthRequest struct {
	Base
	State        string    `gorm:"size:128;uniqueIndex;not null"`
	Nonce        string    `gorm:"size:128;not null"`
	CodeVerifier string    `gorm:"size:128;not null"`
	ExpiresAt    time.Time `gorm:"index;not null"`
}

type Organization struct {
	Base
	Name        string         `gorm:"size:120;not null"`
	Slug        string         `gorm:"size:63;uniqueIndex;not null"`
	Description string         `gorm:"size:1024"`
	Settings    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
}

type Membership struct {
	Base
	OrgID      uuid.UUID `gorm:"type:uuid;index;not null"`
	UserID     uuid.UUID `gorm:"type:uuid;index;not null"`
	Role       string    `gorm:"size:32;not null"`
	Permission string    `gorm:"size:1024"`
}

type Group struct {
	Base
	OrgID uuid.UUID `gorm:"type:uuid;index;not null"`
	Name  string    `gorm:"size:120;not null"`
}

type GroupMember struct {
	Base
	OrgID   uuid.UUID `gorm:"type:uuid;index;not null;uniqueIndex:idx_group_member"`
	GroupID uuid.UUID `gorm:"type:uuid;index;not null;uniqueIndex:idx_group_member"`
	UserID  uuid.UUID `gorm:"type:uuid;index;not null;uniqueIndex:idx_group_member"`
}

type GroupPermission struct {
	Base
	OrgID       uuid.UUID `gorm:"type:uuid;index;not null;uniqueIndex:idx_group_permission"`
	GroupID     uuid.UUID `gorm:"type:uuid;index;not null;uniqueIndex:idx_group_permission"`
	Permission  string    `gorm:"size:120;index;not null;uniqueIndex:idx_group_permission"`
	Description string    `gorm:"size:512"`
}

type Permission struct {
	Base
	OrgID      uuid.UUID `gorm:"type:uuid;index;not null"`
	Name       string    `gorm:"size:120;not null"`
	Resource   string    `gorm:"size:128;not null"`
	Action     string    `gorm:"size:64;not null"`
	IsBuiltIn  bool      `gorm:"not null;default:false"`
	Definition string    `gorm:"size:2048"`
}

type OIDCConfig struct {
	Base
	OrgID         uuid.UUID `gorm:"type:uuid;uniqueIndex;not null"`
	IssuerURL     string    `gorm:"size:2048;not null"`
	ClientID      string    `gorm:"size:255;not null"`
	ClientSecret  string    `gorm:"size:255;not null"`
	GroupClaim    string    `gorm:"size:128;not null;default:groups"`
	UsernameClaim string    `gorm:"size:128;not null;default:email"`
	Enabled       bool      `gorm:"not null;default:false"`
}

type OIDCMapping struct {
	Base
	OrgID            uuid.UUID `gorm:"type:uuid;index;not null"`
	SubjectType      string    `gorm:"size:32;not null;default:group"`
	ExternalValue    string    `gorm:"size:255;not null"`
	ExternalGroup    string    `gorm:"size:255;not null"`
	MappedRole       string    `gorm:"size:32;not null"`
	CustomPermission string    `gorm:"size:120"`
}

type OrgNamespace struct {
	Base
	OrgID     uuid.UUID `gorm:"type:uuid;index;not null"`
	Namespace string    `gorm:"size:63;uniqueIndex;not null"`
	Cluster   string    `gorm:"size:120;not null;default:default"`
	CreatedBy uuid.UUID `gorm:"type:uuid;index;not null"`
}

type AuditEvent struct {
	Base
	OrgID       uuid.UUID      `gorm:"type:uuid;index;not null"`
	ActorUserID uuid.UUID      `gorm:"type:uuid;index;not null"`
	EventType   string         `gorm:"size:120;index;not null"`
	Resource    string         `gorm:"size:120;index;not null"`
	Status      string         `gorm:"size:32;index;not null"`
	Message     string         `gorm:"size:1024;not null"`
	Details     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
}

type ManagedResource struct {
	Base
	OrgID         uuid.UUID      `gorm:"type:uuid;index;not null"`
	Namespace     string         `gorm:"size:63;index;not null"`
	ResourceType  string         `gorm:"size:64;index;not null"`
	ResourceName  string         `gorm:"size:253;not null"`
	Cluster       string         `gorm:"size:120;not null;default:default"`
	Spec          datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastAppliedAt *time.Time
}

func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
