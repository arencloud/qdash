package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/arencloud/qdash/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const SessionTTL = 12 * time.Hour

type AuthService struct {
	db *gorm.DB
}

func NewAuthService(db *gorm.DB) *AuthService {
	return &AuthService{db: db}
}

func (s *AuthService) EnsureOIDCUser(email, displayName string) (models.User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return models.User{}, fmt.Errorf("email claim is required")
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = displayFromEmail(email)
	}

	var user models.User
	err := s.db.Where("email = ?", email).First(&user).Error
	if err == nil {
		user.DisplayName = displayName
		user.Source = "oidc"
		if saveErr := s.db.Save(&user).Error; saveErr != nil {
			return models.User{}, saveErr
		}
		return user, nil
	}
	if err != gorm.ErrRecordNotFound {
		return models.User{}, err
	}
	user = models.User{Email: email, DisplayName: displayName, Source: "oidc"}
	if err := s.db.Create(&user).Error; err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (s *AuthService) CreateSession(userID uuid.UUID) (string, error) {
	rawToken, err := randomToken(32)
	if err != nil {
		return "", err
	}
	session := models.AuthSession{
		UserID:    userID,
		TokenHash: models.HashSessionToken(rawToken),
		ExpiresAt: time.Now().Add(SessionTTL),
	}
	if err := s.db.Create(&session).Error; err != nil {
		return "", err
	}
	return rawToken, nil
}

func (s *AuthService) AuthenticateByToken(token string) (models.User, bool, error) {
	if token == "" {
		return models.User{}, false, nil
	}
	var session models.AuthSession
	err := s.db.Where("token_hash = ?", models.HashSessionToken(token)).First(&session).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return models.User{}, false, nil
		}
		return models.User{}, false, err
	}
	if time.Now().After(session.ExpiresAt) {
		_ = s.db.Delete(&session).Error
		return models.User{}, false, nil
	}
	var user models.User
	if err := s.db.First(&user, "id = ?", session.UserID).Error; err != nil {
		return models.User{}, false, err
	}
	return user, true, nil
}

func (s *AuthService) Logout(token string) error {
	if token == "" {
		return nil
	}
	return s.db.Where("token_hash = ?", models.HashSessionToken(token)).Delete(&models.AuthSession{}).Error
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func displayFromEmail(email string) string {
	for i := 0; i < len(email); i++ {
		if email[i] == '@' {
			if i == 0 {
				return "user"
			}
			return email[:i]
		}
	}
	return email
}
