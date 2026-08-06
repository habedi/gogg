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
	lt, root, _ := newLibraryFixtureInWindow(t, games)
	return lt, root
}

// newLibraryFixtureInWindow is the same, and also hands back the window the
// library was built against: a menu or a dialog the library opens belongs to
// that window's canvas.
func newLibraryFixtureInWindow(t *testing.T, games int) (*libraryTab, string, fyne.Window) {
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

	return LibraryTabUI(win, nil, dm, func() {}), root, win
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
func storeStub(t *testing.T, asked *pathLog) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if asked != nil {
			asked.add(r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// countParses counts how often the stored game data is read.
func countParses(t *testing.T) *atomic.Int64 {
	t.Helper()
	var parses atomic.Int64
	original := parseGameData
	parseGameData = func(data string) (client.Game, error) {
		parses.Add(1)
		return original(data)
	}
	t.Cleanup(func() { parseGameData = original })
	return &parses
}

// A query is asked of every game on every keystroke, and answering "which
// platforms does this offer" means reading the game's stored data. Reading it
// again for every letter made typing crawl in a large library.
func TestLibrary_SearchDoesNotReparseTheCatalogue(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 5)
	parses := countParses(t)

	lt.searchEntry.SetText("platform:windows")
	first := parses.Load()
	require.Positive(t, first, "the first search has to read the catalogue")

	for _, typed := range []string{"platform:windows g", "platform:windows ga", "platform:windows gam"} {
		lt.searchEntry.SetText(typed)
	}

	require.Equal(t, first, parses.Load(), "typing must not read the catalogue again")
}

// A catalogue that has been refreshed is a different catalogue, so what was
// read of the old one has to go.
func TestLibrary_RefreshReadsTheCatalogueAgain(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		refreshes := captureRefreshes(t)
		lt, _ := newLibraryFixture(t, 3)
		lt.searchEntry.SetText("platform:windows")

		parses := countParses(t)
		test.Tap(buttonWithLabel(lt.content, "Refresh"))
		refreshes.finish()
		lt.searchEntry.SetText("platform:windows ")

		require.Positive(t, parses.Load(), "a refreshed catalogue has to be read again")
	})
}

// The box takes filters as well as titles, and nothing on screen said so.
func TestLibraryTab_SearchBoxSaysItTakesFilters(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 1)

	require.Contains(t, lt.searchEntry.PlaceHolder, "platform:",
		"the search box has to hint at what else it takes")
}

// newLibraryFixtureShown is a library that is what its window is showing.
func newLibraryFixtureShown(t *testing.T, games int) (*libraryTab, fyne.Window) {
	t.Helper()
	lt, _, win := newLibraryFixtureInWindow(t, games)
	win.SetContent(lt.content)
	return lt, win
}
