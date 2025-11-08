package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	db.Db = gormDB
	require.NoError(t, db.Db.AutoMigrate(&db.Token{}, &db.Game{}))
}

func TestRefreshToken_Integration_Success(t *testing.T) {
	setupTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/token", r.URL.Path)
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, "expired-refresh-token", r.FormValue("refresh_token"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-shiny-access-token",
			"refresh_token": "new-shiny-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	expiredToken := &db.Token{
		AccessToken:  "expired-access-token",
		RefreshToken: "expired-refresh-token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	}
	require.NoError(t, db.UpsertTokenRecord(expiredToken))

	tokenRepo := db.NewTokenRepository(db.GetDB())
	refresher := &client.GogClient{TokenURL: server.URL + "/token"}
	authService := auth.NewServiceWithRepo(tokenRepo, refresher)

	refreshedToken, err := authService.RefreshToken()

	require.NoError(t, err)
	assert.Equal(t, "new-shiny-access-token", refreshedToken.AccessToken)
	assert.Equal(t, "new-shiny-refresh-token", refreshedToken.RefreshToken)

	dbToken, err := db.GetTokenRecord()
	require.NoError(t, err)
	assert.Equal(t, "new-shiny-access-token", dbToken.AccessToken)
}

func TestRefreshToken_Integration_ApiFailure(t *testing.T) {
	setupTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error_description": "Invalid refresh token",
		})
	}))
	defer server.Close()

	expiredToken := &db.Token{
		AccessToken:  "old-token",
		RefreshToken: "invalid-refresh",
		ExpiresAt:    time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
	}
	require.NoError(t, db.UpsertTokenRecord(expiredToken))

	tokenRepo := db.NewTokenRepository(db.GetDB())
	refresher := &client.GogClient{TokenURL: server.URL + "/token"}
	authService := auth.NewServiceWithRepo(tokenRepo, refresher)

	_, err := authService.RefreshToken()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid refresh token")

	dbToken, err := db.GetTokenRecord()
	require.NoError(t, err)
	assert.Equal(t, "old-token", dbToken.AccessToken, "Token in DB should not have been updated on failure")
}
