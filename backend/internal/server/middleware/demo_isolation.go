package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	demoEndpointUnavailableCode = "DEMO_ENDPOINT_UNAVAILABLE"
	maxDemoBearerTokenLength    = 8192
)

// DemoTokenGuard is mounted globally. A valid demo bearer token may only reach
// /auth/me so the frontend can bootstrap its virtual identity; every other
// request is stopped before it can reach a real public or authenticated handler.
func DemoTokenGuard(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hasValidDemoBearer(c, cfg) && !isAllowedDemoEndpoint(c.Request) {
			AbortWithError(c, http.StatusForbidden, demoEndpointUnavailableCode, "This endpoint is unavailable in demo mode")
			return
		}
		c.Next()
	}
}

// DemoIsolationGuard is a second boundary for JWT-authenticated route groups.
// It prevents an accidentally unguarded route from receiving the virtual user
// even if global middleware ordering changes in a future integration.
func DemoIsolationGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsDemoRequest(c) && !isAllowedDemoEndpoint(c.Request) {
			AbortWithError(c, http.StatusForbidden, demoEndpointUnavailableCode, "This endpoint is unavailable in demo mode")
			return
		}
		c.Next()
	}
}

func isAllowedDemoEndpoint(req *http.Request) bool {
	return req != nil && req.Method == http.MethodGet && req.URL != nil && req.URL.Path == "/api/v1/auth/me"
}

func hasValidDemoBearer(c *gin.Context, cfg *config.Config) bool {
	if c == nil || cfg == nil || strings.TrimSpace(cfg.JWT.Secret) == "" {
		return false
	}
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	tokenString := strings.TrimSpace(parts[1])
	if tokenString == "" || len(tokenString) > maxDemoBearerTokenLength {
		return false
	}

	claims := &service.JWTClaims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{
		jwt.SigningMethodHS256.Name,
		jwt.SigningMethodHS384.Name,
		jwt.SigningMethodHS512.Name,
	}))
	token, err := parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, service.ErrInvalidToken
		}
		return []byte(cfg.JWT.Secret), nil
	})
	if err != nil || !token.Valid {
		return false
	}
	return claims.Demo && claims.UserID == service.DemoUserID
}
