package gui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/client"
	"github.com/stretchr/testify/require"
)

const metadataBody = `{
	"overview": "A game about a game.",
	"size": 1024,
	"_links": {"store": {"href": "https://www.gog.com/game/x"}},
	"_embedded": {"publisher": {"name": "A Publisher"}}
}`

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
	srv, calls := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	cache := newMetadataCache(t.TempDir())
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
func TestMetadataCache_ReusesWhatIsOnDisk(t *testing.T) {
	srv, calls := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	dir := t.TempDir()
	_, err := newMetadataCache(dir).fetch(context.Background(), 7)
	require.NoError(t, err)
	asked := calls.Load()

	// A fresh cache stands in for a fresh run of the app.
	meta, err := newMetadataCache(dir).fetch(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "A Publisher", meta.Publisher)
	require.Equal(t, asked, calls.Load(), "the stored copy must be enough")
}

// A description older than a month is fetched again.
func TestMetadataCache_RefetchesAStaleFile(t *testing.T) {
	srv, calls := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	dir := t.TempDir()
	cache := newMetadataCache(dir)
	_, err := cache.fetch(context.Background(), 7)
	require.NoError(t, err)
	asked := calls.Load()

	old := time.Now().Add(-2 * metadataMaxAge)
	require.NoError(t, os.Chtimes(cache.pathFor(7), old, old))

	_, err = newMetadataCache(dir).fetch(context.Background(), 7)
	require.NoError(t, err)
	require.Greater(t, calls.Load(), asked, "a month-old description is looked up again")
}

// A delisted game leaves the library working and the cache empty.
func TestMetadataCache_DoesNotCacheAFailedLookup(t *testing.T) {
	srv, _ := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	dir := t.TempDir()
	_, err := newMetadataCache(dir).fetch(context.Background(), 999)
	require.Error(t, err)

	entries, _ := os.ReadDir(dir)
	require.Empty(t, entries, "a failed lookup must not leave a file behind")
}

// The answer arrives after the click, and the user may have clicked on.
func TestMetadataCache_LoadDeliversToTheSelectedGame(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	srv, _ := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	cache := newMetadataCache(t.TempDir())

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
	srv, _ := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	cache := newMetadataCache(t.TempDir())

	var failed atomic.Int64
	cache.load(999, func(int) bool { return true }, func(client.GameMetadata) {}, func() { failed.Add(1) })
	require.Eventually(t, func() bool { return failed.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

	var stale atomic.Int64
	cache.load(999, func(int) bool { return false }, func(client.GameMetadata) {}, func() { stale.Add(1) })
	require.Never(t, func() bool { return stale.Load() > 0 }, 300*time.Millisecond, 20*time.Millisecond)
}

func TestMetadataCacheDir(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	require.Equal(t, "metadata", filepath.Base(metadataCacheDir()))
}

// A description cached before gogg read screenshots is not a game without
// screenshots: it is the answer to a question nobody had asked yet. Trusting it
// for a month left games with nothing in the gallery for no reason.
func TestMetadataCache_ReadsAgainWhatAnOlderGoggWrote(t *testing.T) {
	srv, calls := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)

	dir := t.TempDir()
	older, err := json.Marshal(client.GameMetadata{Summary: "what an older gogg knew"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "7.json"), older, 0o644))

	meta, err := newMetadataCache(dir).fetch(context.Background(), 7)

	require.NoError(t, err)
	require.Equal(t, "A Publisher", meta.Publisher, "GOG has to be asked again")
	require.Positive(t, calls.Load())
}

// What this gogg wrote is still read from disk between runs.
func TestMetadataCache_KeepsWhatThisGoggWrote(t *testing.T) {
	srv, calls := metadataAPI(t)
	t.Setenv("GOGG_API_BASE", srv.URL)
	dir := t.TempDir()

	_, err := newMetadataCache(dir).fetch(context.Background(), 7)
	require.NoError(t, err)
	asked := calls.Load()

	// A cache of its own, so nothing is remembered in memory.
	meta, err := newMetadataCache(dir).fetch(context.Background(), 7)

	require.NoError(t, err)
	require.Equal(t, "A Publisher", meta.Publisher)
	require.Equal(t, asked, calls.Load(), "what is on disk is used as it is")
}
