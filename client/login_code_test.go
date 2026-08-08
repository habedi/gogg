package client

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// After logging in, GOG lands the browser on a page whose address carries the
// authorization code. Users paste that address or just the code out of it.
func TestParseAuthCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "full redirect address",
			input: "https://embed.gog.com/on_login_success?origin=client&code=abc123",
			want:  "abc123",
		},
		{
			name:  "code before the other parameters",
			input: "https://embed.gog.com/on_login_success?code=abc123&origin=client",
			want:  "abc123",
		},
		{
			name:  "address pasted with surrounding whitespace",
			input: "  https://embed.gog.com/on_login_success?origin=client&code=abc123\n",
			want:  "abc123",
		},
		{
			name:  "query part only",
			input: "origin=client&code=abc123",
			want:  "abc123",
		},
		{
			name:  "bare code",
			input: "abc123",
			want:  "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAuthCode(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseAuthCode_Rejects(t *testing.T) {
	for _, input := range []string{
		"",
		"   ",
		"https://embed.gog.com/on_login_success?origin=client",
		"https://embed.gog.com/on_login_success?origin=client&code=",
		"paste the address here please",
	} {
		_, err := parseAuthCode(input)
		require.Error(t, err, "input %q", input)
	}
}

// The whole point of the code flow: no browser gogg has to drive.
func TestLoginWithCode_StoresTheTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		require.Equal(t, "authorization_code", r.FormValue("grant_type"))
		require.Equal(t, "abc123", r.FormValue("code"))
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","expires_in":3600}`))
	}))
	defer srv.Close()

	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })

	c := &GogClient{TokenURL: srv.URL}
	require.NoError(t, c.LoginWithCode("https://embed.gog.com/on_login_success?origin=client&code=abc123"))

	stored, err := db.GetTokenRecord()
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, "at", stored.AccessToken)
	require.Equal(t, "rt", stored.RefreshToken)
	require.NotEmpty(t, stored.ExpiresAt)
}

func TestLoginWithCode_ReportsExchangeFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	c := &GogClient{TokenURL: srv.URL}
	require.Error(t, c.LoginWithCode("abc123"))
}

func TestLoginWithCode_RejectsUnusableInput(t *testing.T) {
	c := &GogClient{TokenURL: "http://127.0.0.1:0"}
	require.Error(t, c.LoginWithCode("   "))
}
