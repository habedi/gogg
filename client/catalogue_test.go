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
func (r *stubGameRepo) List(_ context.Context) ([]db.Game, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	games := make([]db.Game, 0, len(r.games))
	for _, g := range r.games {
		games = append(games, g)
	}
	return games, nil
}
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
	_, err := RefreshCatalogue(context.Background(), svc, newStubRepo(), 1, nil)
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
	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), newStubRepo(), 1, func(p float64) {
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
	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
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
	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, func(p float64) {
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
	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
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
	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
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

	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), newStubRepo(), 1, nil)
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
	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 3, func(p float64) {
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

func TestExtractVersion_NilVersions(t *testing.T) {
	g := Game{Downloads: []Downloadable{{
		Language:  "en",
		Platforms: Platform{Windows: []PlatformFile{{Name: "setup.exe"}}},
	}}}
	assert.Equal(t, "", extractVersion(g))
}

func TestExtractVersion_WindowsVersion(t *testing.T) {
	v := "1.2.3"
	g := Game{Downloads: []Downloadable{{
		Language:  "en",
		Platforms: Platform{Windows: []PlatformFile{{Name: "setup.exe", Version: &v}}},
	}}}
	assert.Equal(t, "1.2.3", extractVersion(g))
}

func TestExtractVersion_FallsBackToMac(t *testing.T) {
	v := "2.0"
	g := Game{Downloads: []Downloadable{{
		Language:  "en",
		Platforms: Platform{Mac: []PlatformFile{{Name: "game.dmg", Version: &v}}},
	}}}
	assert.Equal(t, "2.0", extractVersion(g))
}

func TestRefreshCatalogue_VersionChange_NewGame(t *testing.T) {
	v := "1.0"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			json.NewEncoder(w).Encode(map[string]interface{}{"owned": []int{10}})
		case "/account/gameDetails/10.json":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"title": "New Game",
				"downloads": [][]interface{}{
					{"en", map[string]interface{}{
						"windows": []map[string]interface{}{{"name": "setup.exe", "version": &v, "size": "0"}},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	repo := newStubRepo() // empty — game 10 is brand new
	changes, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, 10, changes[0].GameID)
	assert.Equal(t, "New Game", changes[0].Title)
	assert.Equal(t, "", changes[0].OldVersion)
}

func TestRefreshCatalogue_VersionChange_Updated(t *testing.T) {
	newVer := "2.0"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			json.NewEncoder(w).Encode(map[string]interface{}{"owned": []int{20}})
		case "/account/gameDetails/20.json":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"title": "Old Game",
				"downloads": [][]interface{}{
					{"en", map[string]interface{}{
						"windows": []map[string]interface{}{{"name": "setup.exe", "version": &newVer, "size": "0"}},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	// Seed the repo with an older version of game 20.
	repo := newStubRepo()
	_ = repo.Put(context.Background(), db.Game{ID: 20, Title: "Old Game", Version: "1.0"})

	changes, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, "1.0", changes[0].OldVersion)
	assert.Equal(t, "2.0", changes[0].NewVersion)
}

func TestRefreshCatalogue_VersionChange_Removed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GOG no longer lists game 30 as owned.
		if r.URL.Path == "/user/data/games" {
			json.NewEncoder(w).Encode(map[string]interface{}{"owned": []int{}})
		}
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	// Seed repo with game 30 that was previously owned.
	repo := newStubRepo()
	_ = repo.Put(context.Background(), db.Game{ID: 30, Title: "Lost Game", Version: "3.0"})

	changes, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, 30, changes[0].GameID)
	assert.Equal(t, "Lost Game", changes[0].Title)
	assert.Equal(t, "3.0", changes[0].OldVersion)
	assert.Equal(t, "", changes[0].NewVersion)
}

func TestRefreshCatalogue_VersionChange_NoChange(t *testing.T) {
	v := "1.5"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			json.NewEncoder(w).Encode(map[string]interface{}{"owned": []int{40}})
		case "/account/gameDetails/40.json":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"title": "Stable Game",
				"downloads": [][]interface{}{
					{"en", map[string]interface{}{
						"windows": []map[string]interface{}{{"name": "setup.exe", "version": &v, "size": "0"}},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GOGG_EMBED_BASE", server.URL)

	repo := newStubRepo()
	_ = repo.Put(context.Background(), db.Game{ID: 40, Title: "Stable Game", Version: "1.5"})

	changes, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
	require.NoError(t, err)
	assert.Empty(t, changes)
}
