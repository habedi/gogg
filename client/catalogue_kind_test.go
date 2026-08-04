package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

const detailsWithoutVersion = `{"title":"No Version","downloads":[],"extras":[],"dlcs":[]}`

func detailsWithVersion(version string) string {
	return `{"title":"Versioned","downloads":[["English",{"windows":[` +
		`{"manualUrl":"/x","name":"setup.exe","version":"` + version + `","size":"1 MB"}]}]],` +
		`"extras":[],"dlcs":[]}`
}

// Plenty of GOG games carry no version string at all, so the kind of change
// cannot be derived from the version fields afterwards: it has to be recorded where it is known.
func TestRefreshCatalogue_ReportsChangeKinds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			_, _ = w.Write([]byte(`{"owned":[1,2]}`))
		case "/account/gameDetails/1.json":
			_, _ = w.Write([]byte(detailsWithoutVersion)) // brand new, no version
		default:
			_, _ = w.Write([]byte(detailsWithVersion("2.0")))
		}
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	repo := newRealRepo(
		db.Game{ID: 2, Title: "Versioned", Data: "{}", Version: "1.0"},
		db.Game{ID: 99, Title: "Sold", Data: "{}"}, // no version, no longer owned
	)

	changes, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 2, nil)
	require.NoError(t, err)

	byID := make(map[int]VersionChange, len(changes))
	for _, c := range changes {
		byID[c.GameID] = c
	}
	require.Len(t, byID, 3)

	require.Equal(t, ChangeAdded, byID[1].Kind, "a new game without a version is still an addition")
	require.Equal(t, ChangeUpdated, byID[2].Kind)
	require.Equal(t, ChangeRemoved, byID[99].Kind, "a sold game without a version is still a removal")
}

// When the account reports no games at all, nothing is deleted, but the games
// that are no longer listed are still reported as removals.
func TestRefreshCatalogue_EmptyAccountReportsRemovals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"owned":[]}`))
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	repo := newRealRepo(db.Game{ID: 7, Title: "Kept On Disk", Data: "{}"})

	changes, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 1, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, ChangeRemoved, changes[0].Kind)

	all, err := repo.List(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 1, "an empty listing must not wipe the catalogue")
}

// clearRejectingRepo fails the test if anything empties the catalogue.
type clearRejectingRepo struct {
	*realGameRepo
	t *testing.T
}

func (r *clearRejectingRepo) Clear(ctx context.Context) error {
	r.t.Error("RefreshCatalogue must not empty the catalogue")
	return r.realGameRepo.Clear(ctx)
}

// Emptying the catalogue up front is what lost games whenever a refresh failed
// or was cancelled part way through; removals go through DeleteByIDs instead.
func TestRefreshCatalogue_NeverClearsTheCatalogue(t *testing.T) {
	srv := gogServer(t, `{"owned":[1,2]}`, map[string]bool{"/account/gameDetails/2.json": true})
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	repo := &clearRejectingRepo{
		realGameRepo: newRealRepo(db.Game{ID: 1, Title: "One", Data: "{}"}, db.Game{ID: 2, Title: "Two", Data: "{}"}),
		t:            t,
	}

	_, err := RefreshCatalogue(context.Background(), newAuthSvc(validToken(), nil), repo, 2, nil)
	require.NoError(t, err)

	all, err := repo.List(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 2)
}
