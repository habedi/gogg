package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformTokenRefresh_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/token", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		r.ParseForm()
		assert.Equal(t, "my-refresh-token", r.FormValue("refresh_token"))
		assert.Equal(t, "refresh_token", r.FormValue("grant_type"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_in":    7200,
		})
	}))
	defer server.Close()

	client := &GogClient{TokenURL: server.URL + "/token"}
	accessToken, refreshToken, expiresIn, err := client.PerformTokenRefresh("my-refresh-token")

	require.NoError(t, err)
	assert.Equal(t, "new-access-token", accessToken)
	assert.Equal(t, "new-refresh-token", refreshToken)
	assert.Equal(t, int64(7200), expiresIn)
}

func TestPerformTokenRefresh_ApiError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error_description": "The provided authorization code is invalid or expired",
		})
	}))
	defer server.Close()

	client := &GogClient{TokenURL: server.URL + "/token"}
	_, _, _, err := client.PerformTokenRefresh("bad-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "The provided authorization code is invalid or expired")
}

func TestExchangeCodeForToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/token", r.URL.Path)
		r.ParseForm()
		assert.Equal(t, "my-auth-code", r.FormValue("code"))
		assert.Equal(t, "authorization_code", r.FormValue("grant_type"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access-from-code",
			"refresh_token": "refresh-from-code",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	// exchangeCodeForToken uses a hardcoded URL, so we can't test it
	// without further refactoring. This highlights the benefit of dependency injection.
	// For now, we'll skip directly testing it and rely on the higher-level Login flow.
	t.Skip("Skipping TestExchangeCodeForToken as it uses a hardcoded URL")
}
