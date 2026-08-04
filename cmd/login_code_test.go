package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tokenServer(t *testing.T, wantCode string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		assert.Equal(t, "authorization_code", r.FormValue("grant_type"))
		assert.Equal(t, wantCode, r.FormValue("code"))
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","expires_in":3600}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The point of --code: logging in on a machine where no browser can be driven,
// by pasting back the address GOG redirected another browser to.
func TestLoginCmd_CodeFlagStoresTokens(t *testing.T) {
	cleanDBTables(t)
	srv := tokenServer(t, "abc123")

	out, err := captureCombinedOutput(loginCmd(&client.GogClient{TokenURL: srv.URL}),
		"--code", "https://embed.gog.com/on_login_success?origin=client&code=abc123")
	require.NoError(t, err)
	assert.Contains(t, out, "Login was successful")

	token, err := db.GetTokenRecord()
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, "at", token.AccessToken)
	assert.Equal(t, "rt", token.RefreshToken)
	assert.NotEmpty(t, token.ExpiresAt)
}

func TestLoginCmd_CodeFlagAcceptsABareCode(t *testing.T) {
	cleanDBTables(t)
	srv := tokenServer(t, "abc123")

	out, err := captureCombinedOutput(loginCmd(&client.GogClient{TokenURL: srv.URL}), "--code", "abc123")
	require.NoError(t, err)
	assert.Contains(t, out, "Login was successful")

	token, err := db.GetTokenRecord()
	require.NoError(t, err)
	require.NotNil(t, token)
}

// Nothing usable in, nothing stored, and no browser started either.
func TestLoginCmd_CodeFlagRejectsUnusableInput(t *testing.T) {
	cleanDBTables(t)

	out, err := captureCombinedOutput(loginCmd(&client.GogClient{TokenURL: "http://127.0.0.1:1"}),
		"--code", "paste the address here")
	require.NoError(t, err)
	assert.Contains(t, out, "Failed to login to GOG.com")

	token, err := db.GetTokenRecord()
	require.NoError(t, err)
	assert.Nil(t, token)
}

// The flag is useless without knowing which page to open.
func TestLoginCmd_HelpShowsTheLoginURL(t *testing.T) {
	out, err := captureCombinedOutput(loginCmd(&client.GogClient{}), "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "--code")
	assert.Contains(t, out, "auth.gog.com")
}
