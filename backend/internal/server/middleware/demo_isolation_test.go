//go:build unit

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type demoIsolationUserRepo struct {
	service.UserRepository
	getByIDCalls int
}

func (r *demoIsolationUserRepo) GetByID(context.Context, int64) (*service.User, error) {
	r.getByIDCalls++
	return nil, errors.New("unexpected user lookup")
}

func newDemoIsolationAuthService(repo service.UserRepository) (*service.AuthService, *config.Config) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-demo-jwt-secret-at-least-32-bytes",
			AccessTokenExpireMinutes: 60,
		},
		DemoAccount: config.DemoAccountConfig{
			Enabled:  true,
			Email:    "demo@example.test",
			Password: "demo-password",
		},
	}
	return service.NewAuthService(nil, repo, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil), cfg
}

func TestDemoJWTAuthDoesNotReadUserOrTouchActivity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &demoIsolationUserRepo{}
	authService, _ := newDemoIsolationAuthService(repo)
	toucher := &recordingActivityToucher{}

	router := gin.New()
	router.Use(jwtAuth(authService, repo, toucher, nil, nil))
	router.GET("/protected", func(c *gin.Context) {
		demoUser, ok := GetDemoUserFromContext(c)
		require.True(t, ok)
		c.JSON(http.StatusOK, gin.H{"id": demoUser.ID})
	})

	token, err := authService.GenerateToken(context.Background(), authService.DemoUser())
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Zero(t, repo.getByIDCalls)
	require.Empty(t, toucher.userIDs)
}

func TestDemoTokenGuardBlocksPublicWritesButAllowsProfileBootstrap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authService, cfg := newDemoIsolationAuthService(&demoIsolationUserRepo{})
	token, err := authService.GenerateToken(context.Background(), authService.DemoUser())
	require.NoError(t, err)

	writeReached := false
	router := gin.New()
	router.Use(DemoTokenGuard(cfg))
	router.POST("/api/v1/auth/register", func(c *gin.Context) {
		writeReached = true
		c.Status(http.StatusNoContent)
	})
	router.GET("/api/v1/auth/me", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	write := httptest.NewRecorder()
	writeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	writeReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(write, writeReq)
	require.Equal(t, http.StatusForbidden, write.Code)
	require.Contains(t, write.Body.String(), demoEndpointUnavailableCode)
	require.False(t, writeReached)

	profile := httptest.NewRecorder()
	profileReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	profileReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(profile, profileReq)
	require.Equal(t, http.StatusOK, profile.Code)
}

func TestAdminAuthRejectsDemoJWTWithoutUserLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &demoIsolationUserRepo{}
	authService, _ := newDemoIsolationAuthService(repo)
	userService := service.NewUserService(repo, nil, nil, nil)

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAdminAuthMiddleware(authService, userService, nil, nil)))
	router.GET("/admin", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	token, err := authService.GenerateToken(context.Background(), authService.DemoUser())
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "DEMO_ADMIN_FORBIDDEN")
	require.Zero(t, repo.getByIDCalls)
}
