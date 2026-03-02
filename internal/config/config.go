package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	AppName          string
	Env              string
	BindAddress      string
	DatabaseURL      string
	KubeconfigPath   string
	DefaultIstioSet  string
	OIDCIssuerURL    string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
	OIDCScopes       []string
}

func Load() (Config, error) {
	cfg := Config{
		AppName:          getenv("APP_NAME", "qdash"),
		Env:              getenv("APP_ENV", "dev"),
		BindAddress:      getenv("BIND_ADDRESS", ":8080"),
		DatabaseURL:      getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/qdash?sslmode=disable"),
		KubeconfigPath:   os.Getenv("KUBECONFIG"),
		DefaultIstioSet:  getenv("DEFAULT_ISTIO_PROFILE", "default"),
		OIDCIssuerURL:    strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL")),
		OIDCClientID:     strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID")),
		OIDCClientSecret: strings.TrimSpace(os.Getenv("OIDC_CLIENT_SECRET")),
		OIDCRedirectURL:  strings.TrimSpace(os.Getenv("OIDC_REDIRECT_URL")),
		OIDCScopes:       splitCSV(getenv("OIDC_SCOPES", "openid,profile,email,groups")),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL cannot be empty")
	}
	return cfg, nil
}

func splitCSV(in string) []string {
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getenv(name, fallback string) string {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	return v
}
