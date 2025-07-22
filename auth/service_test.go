package auth_test

import (
	"errors"
	"testing"
	"time"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStorer struct {
	tokenToReturn *db.Token
	errToReturn   error
	upsertCalled  bool
}

func (m *mockStorer) GetTokenRecord() (*db.Token, error) {
	return m.tokenToReturn, m.errToReturn
}

func (m *mockStorer) UpsertTokenRecord(token *db.Token) error {
	m.upsertCalled = true
	m.tokenToReturn = token
	return nil
}

type mockRefresher struct {
	errToReturn error
}

func (m *mockRefresher) PerformTokenRefresh(refreshToken string) (string, string, int64, error) {
	if m.errToReturn != nil {
		return "", "", 0, m.errToReturn
	}
	return "new-access-token", "new-refresh-token", 3600, nil
}

func TestRefreshToken_WhenTokenIsValid(t *testing.T) {
	storer := &mockStorer{
		tokenToReturn: &db.Token{
			AccessToken:  "valid-access",
			RefreshToken: "valid-refresh",
			ExpiresAt:    time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		},
	}
	service := auth.NewService(storer, &mockRefresher{})

	token, err := service.RefreshToken()

	require.NoError(t, err)
	assert.Equal(t, "valid-access", token.AccessToken)
	assert.False(t, storer.upsertCalled, "Upsert should not be called for a valid token")
}

func TestRefreshToken_WhenTokenIsExpired(t *testing.T) {
	storer := &mockStorer{
		tokenToReturn: &db.Token{
			AccessToken:  "expired-access",
			RefreshToken: "expired-refresh",
			ExpiresAt:    time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}
	service := auth.NewService(storer, &mockRefresher{})

	token, err := service.RefreshToken()

	require.NoError(t, err)
	assert.Equal(t, "new-access-token", token.AccessToken)
	assert.Equal(t, "new-refresh-token", token.RefreshToken)
	assert.True(t, storer.upsertCalled, "Upsert should be called for an expired token")
}

func TestRefreshToken_WhenRefreshFails(t *testing.T) {
	storer := &mockStorer{
		tokenToReturn: &db.Token{
			AccessToken:  "expired-access",
			RefreshToken: "expired-refresh",
			ExpiresAt:    time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}
	refresher := &mockRefresher{errToReturn: errors.New("network error")}
	service := auth.NewService(storer, refresher)

	_, err := service.RefreshToken()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "network error")
	assert.False(t, storer.upsertCalled, "Upsert should not be called if refresh fails")
}

func TestRefreshToken_WhenNoTokenInDB(t *testing.T) {
	storer := &mockStorer{tokenToReturn: nil}
	service := auth.NewService(storer, &mockRefresher{})

	_, err := service.RefreshToken()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "token record does not exist")
}
