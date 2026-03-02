package middleware

import (
	"net/http"
	"strings"

	"github.com/arencloud/qdash/internal/models"
	"github.com/arencloud/qdash/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	UserContextKey    = "currentUser"
	SessionCookieName = "qdash_session"
)

func UserFromContext(c *gin.Context) (models.User, bool) {
	v, ok := c.Get(UserContextKey)
	if !ok {
		return models.User{}, false
	}
	u, ok := v.(models.User)
	return u, ok
}

func SessionAuth(authSvc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := tokenFromRequest(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		user, ok, err := authSvc.AuthenticateByToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "auth failed"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			return
		}
		c.Set(UserContextKey, user)
		c.Next()
	}
}

func SessionAuthWeb(authSvc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := tokenFromRequest(c)
		if token == "" {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		user, ok, err := authSvc.AuthenticateByToken(token)
		if err != nil || !ok {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Set(UserContextKey, user)
		c.Next()
	}
}

func tokenFromRequest(c *gin.Context) string {
	if token := strings.TrimSpace(c.GetHeader("X-Session-Token")); token != "" {
		return token
	}
	if authz := strings.TrimSpace(c.GetHeader("Authorization")); authz != "" {
		const prefix = "Bearer "
		if strings.HasPrefix(authz, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(authz, prefix))
		}
	}
	token, _ := c.Cookie(SessionCookieName)
	return strings.TrimSpace(token)
}
