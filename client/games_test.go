package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/habedi/gogg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFetchGameData_ReturnsGameData tests that FetchGameData returns the correct game data
// when provided with a valid token and URL.
func TestFetchGameData_ReturnsGameData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"title": "Test Game"}`))
	}))
	defer server.Close()

	game, body, err := client.FetchGameData("valid_token", server.URL)
	require.NoError(t, err)
	assert.Equal(t, "Test Game", game.Title)
	assert.Equal(t, `{"title": "Test Game"}`, body)
}

// TestFetchGameData_ReturnsErrorOnInvalidToken tests that FetchGameData returns an error
// when provided with an invalid token.
func TestFetchGameData_ReturnsErrorOnInvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, _, err := client.FetchGameData("invalid_token", server.URL)
	assert.Error(t, err)
}

// TestFetchIdOfOwnedGames_ReturnsOwnedGames tests that FetchIdOfOwnedGames returns the correct
// list of owned game IDs when provided with a valid token and URL.
func TestFetchIdOfOwnedGames_ReturnsOwnedGames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"owned": [1, 2, 3]}`))
	}))
	defer server.Close()

	ids, err := client.FetchIdOfOwnedGames("valid_token", server.URL)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, ids)
}

// TestFetchIdOfOwnedGames_ReturnsErrorOnInvalidToken tests that FetchIdOfOwnedGames returns an error
// when provided with an invalid token.
func TestFetchIdOfOwnedGames_ReturnsErrorOnInvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := client.FetchIdOfOwnedGames("invalid_token", server.URL)
	assert.Error(t, err)
}

// TestFetchIdOfOwnedGames_ReturnsErrorOnInvalidResponse tests that FetchIdOfOwnedGames returns an error
// when the response from the server is invalid.
func TestFetchIdOfOwnedGames_ReturnsErrorOnInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // Should raise error
		w.Write([]byte(`{"invalid": "response"}`))
	}))
	defer server.Close()

	_, err := client.FetchIdOfOwnedGames("valid_token", server.URL)
	assert.Error(t, err)
}
