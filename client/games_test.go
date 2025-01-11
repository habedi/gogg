package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/habedi/gogg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestFetchGameData_ReturnsErrorOnInvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, _, err := client.FetchGameData("invalid_token", server.URL)
	assert.Error(t, err)
}

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

func TestFetchIdOfOwnedGames_ReturnsErrorOnInvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := client.FetchIdOfOwnedGames("invalid_token", server.URL)
	assert.Error(t, err)
}

func TestFetchIdOfOwnedGames_ReturnsErrorOnInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // Should raise error
		w.Write([]byte(`{"invalid": "response"}`))
	}))
	defer server.Close()

	_, err := client.FetchIdOfOwnedGames("valid_token", server.URL)
	assert.Error(t, err)
}
