package client

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// truncatedPostServer returns a server that drains the request body, then sends
// HTTP 200 headers with Content-Length: 100 and immediately closes the
// connection, causing io.ReadAll(resp.Body) to return an error.
func truncatedPostServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body) //nolint:errcheck
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("httptest server does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\nContent-Type: application/json\r\n\r\n")) //nolint:errcheck
		conn.(*net.TCPConn).CloseWrite()                                                                       //nolint:errcheck
		conn.Close()                                                                                           //nolint:errcheck
	}))
}

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

func TestExtractAuthCode_Valid(t *testing.T) {
	code, err := extractAuthCode("https://embed.gog.com/on_login_success?origin=client&code=abc123")
	require.NoError(t, err)
	assert.Equal(t, "abc123", code)
}

func TestExtractAuthCode_MissingCode(t *testing.T) {
	_, err := extractAuthCode("https://embed.gog.com/on_login_success?origin=client")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authorization code not found")
}

func TestExtractAuthCode_EmptyCode(t *testing.T) {
	_, err := extractAuthCode("https://embed.gog.com/on_login_success?code=")
	require.Error(t, err)
}

func TestPerformTokenRefresh_InvalidJSONResponse(t *testing.T) {
	// 200 with non-JSON body triggers the json.Unmarshal error branch.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := &GogClient{TokenURL: server.URL}
	_, _, _, err := c.PerformTokenRefresh("token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse token refresh response")
}

func TestPerformTokenRefresh_ErrorInBody(t *testing.T) {
	// 200 with a parsed error_description triggers the result.Error branch.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"error_description": "session expired",
		})
	}))
	defer server.Close()

	c := &GogClient{TokenURL: server.URL}
	_, _, _, err := c.PerformTokenRefresh("token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session expired")
}

func TestExchangeCodeForToken_InvalidJSONResponse(t *testing.T) {
	// 200 with non-JSON body triggers the json.Unmarshal error branch.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := &GogClient{TokenURL: server.URL}
	_, _, _, err := c.exchangeCodeForToken("some-code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse token response")
}

func TestPerformTokenRefresh_ReadBodyError(t *testing.T) {
	server := truncatedPostServer(t)
	defer server.Close()

	c := &GogClient{TokenURL: server.URL}
	_, _, _, err := c.PerformTokenRefresh("token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read token refresh response")
}

func TestExchangeCodeForToken_ReadBodyError(t *testing.T) {
	server := truncatedPostServer(t)
	defer server.Close()

	c := &GogClient{TokenURL: server.URL}
	_, _, _, err := c.exchangeCodeForToken("code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read token response")
}

func TestPerformTokenRefresh_PostFormError(t *testing.T) {
	// Malformed URL causes http.PostForm to fail before any network call.
	c := &GogClient{TokenURL: "://invalid"}
	_, _, _, err := c.PerformTokenRefresh("token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to post form for token refresh")
}

func TestExchangeCodeForToken_PostFormError(t *testing.T) {
	// Malformed URL causes http.PostForm to fail before any network call.
	c := &GogClient{TokenURL: "://invalid"}
	_, _, _, err := c.exchangeCodeForToken("code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to exchange code for token")
}

func TestExtractAuthCode_ParseError(t *testing.T) {
	// A null byte in the URL causes url.Parse to return an error.
	_, err := extractAuthCode("\x00")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse URL")
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

	gogClient := &GogClient{TokenURL: server.URL + "/token"}

	accessToken, refreshToken, expiresAt, err := gogClient.exchangeCodeForToken("my-auth-code")

	require.NoError(t, err)
	assert.Equal(t, "access-from-code", accessToken)
	assert.Equal(t, "refresh-from-code", refreshToken)

	expectedExpiry := time.Now().Add(time.Hour)
	actualExpiry, parseErr := time.Parse(time.RFC3339, expiresAt)
	require.NoError(t, parseErr)
	assert.WithinDuration(t, expectedExpiry, actualExpiry, 5*time.Second)
}
