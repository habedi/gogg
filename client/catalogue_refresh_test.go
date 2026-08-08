package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// realGameRepo is an in-memory repository whose Clear and DeleteByIDs actually
// remove rows, so tests can see what a refresh does to a populated catalogue.
type realGameRepo struct {
	mu    sync.Mutex
	games map[int]db.Game
}

func newRealRepo(games ...db.Game) *realGameRepo {
	r := &realGameRepo{games: make(map[int]db.Game)}
	for _, g := range games {
		r.games[g.ID] = g
	}
	return r
}

func (r *realGameRepo) Put(_ context.Context, g db.Game) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.games[g.ID] = g
	return nil
}

func (r *realGameRepo) GetByID(_ context.Context, id int) (*db.Game, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.games[id]
	if !ok {
		return nil, nil
	}
	return &g, nil
}

func (r *realGameRepo) List(_ context.Context) ([]db.Game, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]db.Game, 0, len(r.games))
	for _, g := range r.games {
		out = append(out, g)
	}
	return out, nil
}

func (r *realGameRepo) SearchByTitle(_ context.Context, _ string) ([]db.Game, error) {
	return nil, nil
}

func (r *realGameRepo) Clear(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.games = make(map[int]db.Game)
	return nil
}

func (r *realGameRepo) DeleteByIDs(_ context.Context, ids []int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		delete(r.games, id)
	}
	return nil
}

func (r *realGameRepo) get(t *testing.T, id int) db.Game {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.games[id]
	if !ok {
		t.Fatalf("game %d is missing from the catalogue", id)
	}
	return g
}

// gogServer serves the owned-games listing plus per-game details. Games listed
// in broken return a 200 with an unparseable body.
func gogServer(t *testing.T, ownedBody string, broken map[string]bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/data/games" {
			_, _ = w.Write([]byte(ownedBody))
			return
		}
		if broken[r.URL.Path] {
			_, _ = w.Write([]byte(`{{{ not json`))
			return
		}
		_, _ = w.Write([]byte(`{"title":"Fresh Title","downloads":[],"extras":[],"dlcs":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A game whose details cannot be fetched must keep the data already in the
// catalogue instead of vanishing from it.
func TestRefreshCatalogue_KeepsGamesWhoseDetailsFail(t *testing.T) {
	srv := gogServer(t, `{"owned":[1,2,3]}`, map[string]bool{"/account/gameDetails/2.json": true})
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	repo := newRealRepo(
		db.Game{ID: 1, Title: "Old One", Data: `{"title":"Old One"}`, Version: "1.0"},
		db.Game{ID: 2, Title: "Old Two", Data: `{"title":"Old Two"}`, Version: "2.0"},
		db.Game{ID: 3, Title: "Old Three", Data: `{"title":"Old Three"}`, Version: "3.0"},
	)

	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 2, nil)
	require.NoError(t, err)

	all, err := repo.List(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 3, "no game may disappear because of a failed fetch")

	// The two that were fetched are updated, the third keeps its old data.
	require.Equal(t, "Fresh Title", repo.get(t, 1).Title)
	require.Equal(t, "Old Two", repo.get(t, 2).Title)
	require.Equal(t, `{"title":"Old Two"}`, repo.get(t, 2).Data)
	require.Equal(t, "Fresh Title", repo.get(t, 3).Title)
}

// Games sold or removed from the GOG account still leave the catalogue.
func TestRefreshCatalogue_RemovesGamesNoLongerOwned(t *testing.T) {
	srv := gogServer(t, `{"owned":[1]}`, nil)
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	repo := newRealRepo(
		db.Game{ID: 1, Title: "Kept", Data: "{}", Version: "1.0"},
		db.Game{ID: 99, Title: "Sold", Data: "{}", Version: "1.0"},
	)

	changes, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 2, nil)
	require.NoError(t, err)

	all, err := repo.List(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, 1, all[0].ID)

	var removed *VersionChange
	for i, c := range changes {
		if c.GameID == 99 {
			removed = &changes[i]
		}
	}
	require.NotNil(t, removed, "the sold game should be reported")
	require.Equal(t, "1.0", removed.OldVersion)
	require.Empty(t, removed.NewVersion)
}

// If the owned-games listing cannot be fetched, the catalogue is left alone.
func TestRefreshCatalogue_ListingFailureLeavesCatalogueIntact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	repo := newRealRepo(
		db.Game{ID: 1, Title: "One", Data: "{}"},
		db.Game{ID: 2, Title: "Two", Data: "{}"},
	)

	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 2, nil)
	require.Error(t, err)

	all, listErr := repo.List(context.Background())
	require.NoError(t, listErr)
	require.Len(t, all, 2, "a failed refresh must not empty the catalogue")
}

// Cancelling a refresh half way through must not leave a gutted catalogue.
func TestRefreshCatalogue_CancellationLeavesCatalogueIntact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/data/games" {
			_, _ = w.Write([]byte(`{"owned":[1,2,3]}`))
			return
		}
		cancel() // the user hits Ctrl-C while game details are being fetched
		_, _ = w.Write([]byte(`{"title":"Fresh Title","downloads":[],"extras":[],"dlcs":[]}`))
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	repo := newRealRepo(
		db.Game{ID: 1, Title: "One", Data: "{}"},
		db.Game{ID: 2, Title: "Two", Data: "{}"},
		db.Game{ID: 3, Title: "Three", Data: "{}"},
	)

	_, err := RefreshCatalogue(ctx, newAuthSvc(validToken(), nil), repo, 1, nil)
	require.Error(t, err)

	all, listErr := repo.List(context.Background())
	require.NoError(t, listErr)
	require.Len(t, all, 3, "a cancelled refresh must not drop games")
}

// The catalogue records where each game's artwork lives, so the GUI does not
// have to ask GOG again every time it draws the library.
func TestRefreshCatalogue_RecordsArtwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			_, _ = w.Write([]byte(`{"owned":[1]}`))
		case "/account/getFilteredProducts":
			_, _ = w.Write([]byte(`{"totalPages":1,"products":[{"id":1,"image":"//images-1.gog-statics.com/aaa"}]}`))
		default:
			_, _ = w.Write([]byte(`{"title":"One","downloads":[],"extras":[],"dlcs":[]}`))
		}
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	repo := newRealRepo()
	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
	require.NoError(t, err)

	require.Equal(t, "//images-1.gog-statics.com/aaa", repo.get(t, 1).CoverImage)
}

// Artwork is decoration: failing to fetch it must not fail a refresh.
func TestRefreshCatalogue_SurvivesMissingArtwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			_, _ = w.Write([]byte(`{"owned":[1]}`))
		case "/account/getFilteredProducts":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte(`{"title":"One","downloads":[],"extras":[],"dlcs":[]}`))
		}
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	repo := newRealRepo()
	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
	require.NoError(t, err, "a refresh must not fail because artwork could not be listed")
	require.Equal(t, "One", repo.get(t, 1).Title)
}
