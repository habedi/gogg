package gui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

const metadataBody = `{
	"overview": "A game about a game.",
	"size": 1024,
	"_links": {"store": {"href": "https://www.gog.com/game/x"}},
	"_embedded": {"publisher": {"name": "A Publisher"}}
}`

// openTestDB gives a test a catalogue database of its own: the cache stores
// what it fetched in there.
func openTestDB(t *testing.T) {
	t.Helper()
	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })
}

// metadataAPI serves the two store endpoints and counts what was asked for.
func metadataAPI(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/v2/games/7":
			_, _ = w.Write([]byte(metadataBody))
		case "/products/7":
			_, _ = w.Write([]byte(`{"release_date": "2020-01-02T00:00:00+0000"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// Clicking back and forth between two games must not hit GOG every time.
func TestMetadataCache_FetchesOncePerGame(t *testing.T) {
	openTestDB(t)
	srv, calls := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	cache := newMetadataCache()
	first, err := cache.fetch(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "A Publisher", first.Publisher)
	require.Equal(t, "2020-01-02", first.ReleaseDate)

	asked := calls.Load()
	second, err := cache.fetch(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, asked, calls.Load(), "the second lookup must come from memory")
}

// Store copy barely changes, so the next run of gogg should not re-fetch it.
func TestMetadataCache_ReusesWhatTheDatabaseHolds(t *testing.T) {
	openTestDB(t)
	srv, calls := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	_, err := newMetadataCache().fetch(context.Background(), 7)
	require.NoError(t, err)
	asked := calls.Load()

	// A fresh cache stands in for a fresh run of the app.
	meta, err := newMetadataCache().fetch(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "A Publisher", meta.Publisher)
	require.Equal(t, asked, calls.Load(), "the stored copy must be enough")
}

// A description older than a month is fetched again.
func TestMetadataCache_RefetchesAStaleRecord(t *testing.T) {
	openTestDB(t)
	srv, calls := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	stale, err := json.Marshal(client.GameMetadata{Summary: "what an older run knew"})
	require.NoError(t, err)
	require.NoError(t, db.PutGameMetadataAt(context.Background(), 7, metadataFormat, stale,
		time.Now().Add(-2*metadataMaxAge)))

	meta, err := newMetadataCache().fetch(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "A Publisher", meta.Publisher, "a month-old description is looked up again")
	require.Positive(t, calls.Load())
}

// A delisted game leaves the library working and the database without a record.
func TestMetadataCache_DoesNotStoreAFailedLookup(t *testing.T) {
	openTestDB(t)
	srv, _ := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	_, err := newMetadataCache().fetch(context.Background(), 999)
	require.Error(t, err)

	record, err := db.GetGameMetadata(context.Background(), 999)
	require.NoError(t, err)
	require.Nil(t, record, "a failed lookup must not leave a record behind")
}

// The answer arrives after the click, and the user may have clicked on.
func TestMetadataCache_LoadDeliversToTheSelectedGame(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	openTestDB(t)
	srv, _ := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	cache := newMetadataCache()

	var delivered atomic.Int64
	cache.load(7, func(int) bool { return true }, func(client.GameMetadata) { delivered.Add(1) }, nil)
	require.Eventually(t, func() bool { return delivered.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

	var stale atomic.Int64
	cache.load(7, func(int) bool { return false }, func(client.GameMetadata) { stale.Add(1) }, nil)
	require.Never(t, func() bool { return stale.Load() > 0 }, 300*time.Millisecond, 20*time.Millisecond)
}

// A lookup that comes back with nothing says so, unless the user has clicked
// on to another game in the meantime.
func TestMetadataCache_LoadReportsAFailedLookup(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	openTestDB(t)
	srv, _ := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	cache := newMetadataCache()

	var failed atomic.Int64
	cache.load(999, func(int) bool { return true }, func(client.GameMetadata) {}, func() { failed.Add(1) })
	require.Eventually(t, func() bool { return failed.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

	var stale atomic.Int64
	cache.load(999, func(int) bool { return false }, func(client.GameMetadata) {}, func() { stale.Add(1) })
	require.Never(t, func() bool { return stale.Load() > 0 }, 300*time.Millisecond, 20*time.Millisecond)
}

// A record written before gogg read screenshots is not a game without
// screenshots: it is the answer to a question nobody had asked yet. Trusting
// it for a month left games with nothing in the gallery for no reason.
func TestMetadataCache_ReadsAgainWhatAnOlderGoggWrote(t *testing.T) {
	openTestDB(t)
	srv, calls := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	older, err := json.Marshal(client.GameMetadata{Summary: "what an older gogg knew"})
	require.NoError(t, err)
	require.NoError(t, db.PutGameMetadata(context.Background(), 7, metadataFormat-1, older))

	meta, err := newMetadataCache().fetch(context.Background(), 7)

	require.NoError(t, err)
	require.Equal(t, "A Publisher", meta.Publisher, "GOG has to be asked again")
	require.Positive(t, calls.Load())
}

// What a lookup stores is readable as the database record it is.
func TestMetadataCache_StoresTheLookupInTheDatabase(t *testing.T) {
	openTestDB(t)
	srv, _ := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	_, err := newMetadataCache().fetch(context.Background(), 7)
	require.NoError(t, err)

	record, err := db.GetGameMetadata(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, metadataFormat, record.Format)

	var meta client.GameMetadata
	require.NoError(t, json.Unmarshal(record.Data, &meta))
	require.Equal(t, "A Publisher", meta.Publisher)
}
