package gui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// newLibraryFixture sets up a logged-in catalogue of downloaded games and
// returns the library tab plus the download root.
func newLibraryFixture(t *testing.T, games int) (*libraryTab, string) {
	t.Helper()

	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })
	require.NoError(t, db.UpsertTokenRecord(&db.Token{
		AccessToken: "a", RefreshToken: "r",
		ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339),
	}))

	blob := `{"title":"G","downloads":[],"extras":[],"dlcs":[]}`
	root := t.TempDir()
	for i := 1; i <= games; i++ {
		title := fmt.Sprintf("Game %d", i)
		require.NoError(t, db.PutInGame(i, title, blob))
		dir := filepath.Join(root, client.SanitizePath(title))
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(blob), 0o644))
	}
	fyne.CurrentApp().Preferences().SetString("lastUsedDownloadPath", root)

	// Caches are per-test: a run must never inherit artwork or descriptions
	// stored by another one.
	cache := t.TempDir()
	original := cacheRoot
	cacheRoot = func() string { return cache }
	t.Cleanup(func() { cacheRoot = original })

	// No test may reach the real store API. A test that cares what was asked
	// for sets GOGG_API_BASE itself before calling this.
	if os.Getenv("GOGG_API_BASE") == "" {
		t.Setenv("GOGG_API_BASE", storeStub(t, nil))
	}

	updateStatusCache = make(map[int]updateStatus)
	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	return LibraryTabUI(win, nil, dm, func() {}), root
}

// Which games are shown depends on the search term; whether a game is
// downloaded does not. Rescanning the filesystem on every keystroke made
// typing in the search box block the UI for hundreds of milliseconds.
func TestLibrary_SearchDoesNotRescanDownloadStatus(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, root := newLibraryFixture(t, 3)

	require.True(t, isGameDownloadedCached(1), "startup must work out the download status")
	require.True(t, isGameDownloadedCached(3))

	// Removing the files makes any filesystem rescan observable.
	require.NoError(t, os.RemoveAll(root))
	lt.searchEntry.SetText("Game 1")

	require.True(t, isGameDownloadedCached(1), "searching must not rescan the filesystem")
	require.True(t, isGameDownloadedCached(3), "a filtered-out game must keep its status")
}

// storeStub stands in for GOG's store API. It answers nothing, so the details
// pane stays with the facts gogg already holds; asked records what was
// requested, for tests that check a lookup happened at all.
func storeStub(t *testing.T, asked *atomic.Int64) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if asked != nil {
			asked.Add(1)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}
