//go:build unit

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type demoLoginUserRepo struct {
	service.UserRepository
	getByEmailCalls int
}

func (r *demoLoginUserRepo) GetByEmail(context.Context, string) (*service.User, error) {
	r.getByEmailCalls++
	return nil, errors.New("unexpected user lookup")
}

func TestAuthServiceDemoLoginDoesNotUseUserRepositoryOrRefreshTokens(t *testing.T) {
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
	repo := &demoLoginUserRepo{}
	authService := service.NewAuthService(nil, repo, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)

	token, user, err := authService.Login(context.Background(), "DEMO@example.test", "demo-password")
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.NotNil(t, user)
	require.True(t, user.IsDemo)
	require.Equal(t, service.DemoUserID, user.ID)
	require.Zero(t, repo.getByEmailCalls)

	claims, err := authService.ValidateToken(token)
	require.NoError(t, err)
	require.True(t, claims.Demo)
	require.Equal(t, service.DemoUserID, claims.UserID)

	_, err = authService.GenerateTokenPair(context.Background(), user, "")
	require.ErrorIs(t, err, service.ErrDemoAccountRestricted)
	require.ErrorIs(t, authService.RevokeAllUserSessions(context.Background(), service.DemoUserID), service.ErrDemoAccountRestricted)
	require.ErrorIs(t, authService.RevokeAllUserTokens(context.Background(), service.DemoUserID), service.ErrDemoAccountRestricted)
}
