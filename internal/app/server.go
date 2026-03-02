package app

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/egevorky/qdash/docs"
	"github.com/egevorky/qdash/internal/config"
	"github.com/egevorky/qdash/internal/db"
	api "github.com/egevorky/qdash/internal/handlers/api"
	webh "github.com/egevorky/qdash/internal/handlers/web"
	"github.com/egevorky/qdash/internal/kube"
	"github.com/egevorky/qdash/internal/middleware"
	"github.com/egevorky/qdash/internal/rbac"
	"github.com/egevorky/qdash/internal/service"
	"github.com/egevorky/qdash/internal/version"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type Config = config.Config

func LoadConfig() (Config, error) {
	return config.Load()
}

func NewServer(cfg Config) (http.Handler, error) {
	database, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	kubeClient, err := kube.NewClient(cfg.KubeconfigPath)
	if err != nil {
		return nil, err
	}

	r := gin.Default()
	templatesGlob := filepath.Join("web", "templates", "**", "*.html")
	r.LoadHTMLGlob(templatesGlob)
	r.Static("/static", "web/static")

	rbacSvc := rbac.NewService(database)
	authSvc := service.NewAuthService(database)
	oidcSvc, err := service.NewOIDCService(context.Background(), database, service.OIDCConfig{
		IssuerURL:    cfg.OIDCIssuerURL,
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		RedirectURL:  cfg.OIDCRedirectURL,
		Scopes:       cfg.OIDCScopes,
	})
	if err != nil {
		return nil, err
	}
	kubeSvc := service.NewResourceService(kube.NewResourceService(kubeClient))

	webHandler := webh.NewHandler(rbacSvc, authSvc, oidcSvc, kubeSvc)
	webHandler.RegisterPublic(r)

	protectedWeb := r.Group("/")
	protectedWeb.Use(middleware.SessionAuthWeb(authSvc))
	webHandler.RegisterProtected(protectedWeb)

	apiV1 := r.Group("/api/v1")
	apiV1.Use(middleware.SessionAuth(authSvc))
	api.NewHandler(database, rbacSvc, kubeSvc).Register(apiV1)

	docs.SwaggerInfo.Title = "QDash API"
	docs.SwaggerInfo.Version = version.Version
	docs.SwaggerInfo.Host = trimHost(cfg.BindAddress)
	docs.SwaggerInfo.BasePath = "/"
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"version":   version.Version,
			"commit":    version.Commit,
			"buildDate": version.BuildDate,
		})
	})

	return r, nil
}

func trimHost(bind string) string {
	if len(bind) > 0 && bind[0] == ':' {
		return fmt.Sprintf("localhost%s", bind)
	}
	return bind
}
