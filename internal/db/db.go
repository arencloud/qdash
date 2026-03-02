package db

import (
	"fmt"

	"github.com/egevorky/qdash/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Open(databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
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
		&models.ManagedResource{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}
