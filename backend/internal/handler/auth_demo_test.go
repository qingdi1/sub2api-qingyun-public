//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type demoHandlerUserRepo struct {
	service.UserRepository
	getByEmailCalls int
}

func (r *demoHandlerUserRepo) GetByEmail(context.Context, string) (*service.User, error) {
	r.getByEmailCalls++
	return nil, errors.New("unexpected user lookup")
}

func newDemoAuthHandler(repo service.UserRepository) (*AuthHandler, *service.AuthService) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-demo-jwt-secret-at-least-32-bytes",
			AccessTokenExpireMinutes: 60,
		},
		DemoAccount: config.DemoAccountConfig{
			Enabled:     true,
			Email:       "demo@example.test",
			Password:    "demo-password",
			DisplayName: "Demo Tester",
		},
	}
	authService := service.NewAuthService(nil, repo, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	return &AuthHandler{cfg: cfg, authService: authService}, authService
}

func TestDemoLoginBypassesNormalLoginChainWithoutRepositoryAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &demoHandlerUserRepo{}
	h, _ := newDemoAuthHandler(repo)
	normalHandlerReached := false

	router := gin.New()
	router.POST("/api/v1/auth/login", h.DemoLoginBypass(), func(c *gin.Context) {
		normalHandlerReached = true
		c.Status(http.StatusInternalServerError)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"demo@example.test","password":"demo-password"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, normalHandlerReached)
	require.Zero(t, repo.getByEmailCalls)
	require.Contains(t, w.Body.String(), `"is_demo":true`)
	require.NotContains(t, w.Body.String(), `"refresh_token"`)
}

func TestDemoCurrentUserUsesVirtualContextWithoutUserService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, authService := newDemoAuthHandler(&demoHandlerUserRepo{})
	demoUser := authService.DemoUser()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: demoUser.ID, Concurrency: demoUser.Concurrency})
	middleware2.SetDemoUserInContext(c, demoUser)
	h.GetCurrentUser(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, true, body.Data["is_demo"])
	require.Equal(t, float64(service.DemoUserID), body.Data["id"])
}
