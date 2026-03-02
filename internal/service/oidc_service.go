package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/egevorky/qdash/internal/models"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const oidcRequestTTL = 10 * time.Minute

type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

type OIDCService struct {
	db       *gorm.DB
	enabled  bool
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth2   oauth2.Config
}

type OIDCIdentity struct {
	Email       string
	DisplayName string
	Subject     string
	RawClaims   map[string]any
}

func NewOIDCService(ctx context.Context, db *gorm.DB, cfg OIDCConfig) (*OIDCService, error) {
	if strings.TrimSpace(cfg.IssuerURL) == "" || strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" || strings.TrimSpace(cfg.RedirectURL) == "" {
		return &OIDCService{db: db, enabled: false}, nil
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email", "groups"}
	}

	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("init oidc provider: %w", err)
	}
	oauthCfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		Scopes:       scopes,
	}
	return &OIDCService{
		db:       db,
		enabled:  true,
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauth2:   oauthCfg,
	}, nil
}

func (s *OIDCService) Enabled() bool {
	return s != nil && s.enabled
}

func (s *OIDCService) AuthCodeURL() (string, error) {
	if !s.Enabled() {
		return "", fmt.Errorf("oidc is not configured")
	}
	state, err := randomURLSafe(32)
	if err != nil {
		return "", err
	}
	nonce, err := randomURLSafe(32)
	if err != nil {
		return "", err
	}
	verifier, err := randomURLSafe(64)
	if err != nil {
		return "", err
	}
	challenge := pkceChallenge(verifier)
	request := models.OIDCAuthRequest{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: verifier,
		ExpiresAt:    time.Now().Add(oidcRequestTTL),
	}
	if err := s.db.Create(&request).Error; err != nil {
		return "", err
	}
	url := s.oauth2.AuthCodeURL(
		state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	return url, nil
}

func (s *OIDCService) ExchangeCallback(ctx context.Context, state, code string) (OIDCIdentity, error) {
	if !s.Enabled() {
		return OIDCIdentity{}, fmt.Errorf("oidc is not configured")
	}
	var req models.OIDCAuthRequest
	if err := s.db.Where("state = ?", state).First(&req).Error; err != nil {
		return OIDCIdentity{}, err
	}
	defer func() {
		_ = s.db.Delete(&req).Error
	}()
	if time.Now().After(req.ExpiresAt) {
		return OIDCIdentity{}, fmt.Errorf("oidc state expired")
	}

	token, err := s.oauth2.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", req.CodeVerifier))
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("oauth exchange failed: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return OIDCIdentity{}, fmt.Errorf("id_token missing in callback")
	}
	idToken, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("verify id_token failed: %w", err)
	}
	claims := struct {
		Subject        string         `json:"sub"`
		Email          string         `json:"email"`
		PreferredName  string         `json:"preferred_username"`
		Name           string         `json:"name"`
		EmailVerified  *bool          `json:"email_verified"`
		Nonce          string         `json:"nonce"`
		AdditionalJSON map[string]any `json:"-"`
	}{}
	if err := idToken.Claims(&claims); err != nil {
		return OIDCIdentity{}, err
	}
	if claims.Nonce != "" && claims.Nonce != req.Nonce {
		return OIDCIdentity{}, fmt.Errorf("oidc nonce mismatch")
	}
	display := strings.TrimSpace(claims.Name)
	if display == "" {
		display = strings.TrimSpace(claims.PreferredName)
	}
	if display == "" {
		display = claims.Email
	}
	if strings.TrimSpace(claims.Email) == "" {
		return OIDCIdentity{}, fmt.Errorf("email claim not found")
	}

	rawClaims := map[string]any{}
	_ = idToken.Claims(&rawClaims)
	return OIDCIdentity{
		Email:       strings.ToLower(strings.TrimSpace(claims.Email)),
		DisplayName: display,
		Subject:     claims.Subject,
		RawClaims:   rawClaims,
	}, nil
}

func randomURLSafe(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
