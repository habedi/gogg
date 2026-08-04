package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// GOG rejects the authorization code: the exchange must report an error
// instead of handing back an empty token.
func TestExchangeCodeForToken_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"bad code"}`))
	}))
	defer srv.Close()

	c := &GogClient{TokenURL: srv.URL}
	access, refresh, _, err := c.exchangeCodeForToken("some-code")
	if err == nil {
		t.Fatalf("expected an error, got access=%q refresh=%q", access, refresh)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention the status code, got: %v", err)
	}
}

// A 200 response without an access token is still a failure.
func TestExchangeCodeForToken_MissingAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"expires_in":3600}`))
	}))
	defer srv.Close()

	c := &GogClient{TokenURL: srv.URL}
	if _, _, _, err := c.exchangeCodeForToken("some-code"); err == nil {
		t.Fatal("expected an error for a response with no access token")
	}
}
