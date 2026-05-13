package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles ---

type stubTokenStore struct {
	token *db.Token
	err   error
}

func (s *stubTokenStore) GetTokenRecord() (*db.Token, error)  { return s.token, s.err }
func (s *stubTokenStore) UpsertTokenRecord(_ *db.Token) error { return nil }

type stubRefresher struct {
	access  string
	refresh string
	err     error
}

func (s *stubRefresher) PerformTokenRefresh(_ string) (string, string, int64, error) {
	return s.access, s.refresh, 3600, s.err
}

type stubGameRepo struct {
	mu       sync.Mutex
	games    map[int]db.Game
	clearErr error
}

func newStubRepo() *stubGameRepo { return &stubGameRepo{games: make(map[int]db.Game)} }

func (r *stubGameRepo) Put(_ context.Context, g db.Game) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.games[g.ID] = g
	return nil
}
func (r *stubGameRepo) GetByID(_ context.Context, id int) (*db.Game, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.games[id]
	if !ok {
		return nil, nil
	}
	return &g, nil
}
func (r *stubGameRepo) List(_ context.Context) ([]db.Game, error)                    { return nil, nil }
func (r *stubGameRepo) SearchByTitle(_ context.Context, _ string) ([]db.Game, error) { return nil, nil }
func (r *stubGameRepo) Clear(_ context.Context) error                                { return r.clearErr }

func validToken() *db.Token {
	return &db.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour).Format(time.RFC3339),
	}
}

func newAuthSvc(token *db.Token, refreshErr error) *auth.Service {
	return auth.NewService(
		&stubTokenStore{token: token},
		&stubRefresher{access: "access", refresh: "refresh", err: refreshErr},
	)
}

// --- embedBase ---

func TestEmbedBase_Default(t *testing.T) {
	t.Setenv("GOGG_EMBED_BASE", "")
	assert.Equal(t, "https://embed.gog.com", embedBase())
}

func TestEmbedBase_EnvOverride(t *testing.T) {
	t.Setenv("GOGG_EMBED_BASE", "http://localhost:9999")
	assert.Equal(t, "http://localhost:9999", embedBase())
}

// --- RefreshCatalogue ---

func TestRefreshCatalogue_TokenRefreshError(t *testing.T) {
	// Empty token forces a refresh; refresher returns an error.
	svc := auth.NewService(
		&stubTokenStore{token: &db.Token{}},
		&stubRefresher{err: errors.New("auth failed")},
	)
	err := RefreshCatalogue(context.Background(), svc, newStubRepo(), 1, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to refresh token")
}

func TestRefreshCatalogue_NoGames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"owned": []int{}})
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	var got float64
	err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), newStubRepo(), 1, func(p float64) {
		got = p
	})
	require.NoError(t, err)
	assert.Equal(t, 1.0, got)
}

func TestRefreshCatalogue_ClearError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"owned": []int{1}})
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	repo := newStubRepo()
	repo.clearErr = errors.New("db error")
	err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to empty catalogue")
}

func TestRefreshCatalogue_StoresGames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			json.NewEncoder(w).Encode(map[string]interface{}{"owned": []int{42}})
		case "/account/gameDetails/42.json":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"title":     "My Game",
				"downloads": [][]interface{}{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	repo := newStubRepo()
	var progress []float64
	err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, func(p float64) {
		progress = append(progress, p)
	})
	require.NoError(t, err)

	g, err := repo.GetByID(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, g)
	assert.Equal(t, "My Game", g.Title)
	assert.Equal(t, []float64{1.0}, progress)
}

func TestRefreshCatalogue_SkipsGameWithoutTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			json.NewEncoder(w).Encode(map[string]interface{}{"owned": []int{7}})
		case "/account/gameDetails/7.json":
			json.NewEncoder(w).Encode(map[string]interface{}{"downloads": [][]interface{}{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	repo := newStubRepo()
	err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
	require.NoError(t, err)

	g, _ := repo.GetByID(context.Background(), 7)
	assert.Nil(t, g)
}

func TestRefreshCatalogue_FetchGameDataError(t *testing.T) {
	// When fetching details for a game returns an error, the worker logs a warning
	// and returns nil — RefreshCatalogue should still succeed.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			json.NewEncoder(w).Encode(map[string]interface{}{"owned": []int{99}})
		case "/account/gameDetails/99.json":
			w.WriteHeader(http.StatusNotFound) // triggers FetchGameData error
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	repo := newStubRepo()
	err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
	require.NoError(t, err)
	g, _ := repo.GetByID(context.Background(), 99)
	assert.Nil(t, g)
}

func TestRefreshCatalogue_FetchOwnedIDsError(t *testing.T) {
	// Server returns 401 on the owned-games endpoint, causing FetchAllOwnedGameIDs to fail.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), newStubRepo(), 1, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch owned game IDs")
}

func TestRefreshCatalogue_MultipleGames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			json.NewEncoder(w).Encode(map[string]interface{}{"owned": []int{1, 2, 3}})
		case "/account/gameDetails/1.json":
			json.NewEncoder(w).Encode(map[string]interface{}{"title": "Game One", "downloads": [][]interface{}{}})
		case "/account/gameDetails/2.json":
			json.NewEncoder(w).Encode(map[string]interface{}{"title": "Game Two", "downloads": [][]interface{}{}})
		case "/account/gameDetails/3.json":
			json.NewEncoder(w).Encode(map[string]interface{}{"title": "Game Three", "downloads": [][]interface{}{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	repo := newStubRepo()
	var mu sync.Mutex
	var progressValues []float64
	err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 3, func(p float64) {
		mu.Lock()
		progressValues = append(progressValues, p)
		mu.Unlock()
	})
	require.NoError(t, err)
	assert.Len(t, progressValues, 3)

	for _, id := range []int{1, 2, 3} {
		g, err := repo.GetByID(context.Background(), id)
		require.NoError(t, err)
		require.NotNil(t, g, "game %d should be stored", id)
	}
}
