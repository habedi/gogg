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

	blob := `{"title":"G","downloads":[["English",{"windows":[{"manualUrl":"/w","name":"setup.exe","size":"1 GB"}]}]],"extras":[],"dlcs":[]}`
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
	// for sets GOGG_API_BASE itself before calling this. The default store
	// answers nothing until the test is over: an answer landing mid-test
	// writes to the pane on its own goroutine while the test is reading it.
	if os.Getenv("GOGG_API_BASE") == "" {
		t.Setenv("GOGG_API_BASE", hangingStoreStub(t))
	}

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	lt := LibraryTabUI(win, stubAuthService(), dm, openStores(), func() {})
	// Closed before the store stub lets its answers go, so nothing is
	// delivered to widgets the next test cannot see.
	t.Cleanup(lt.close)
	return lt, root, win
}

// Which games are shown depends on the search term; whether a game is
// downloaded does not. Rescanning the filesystem on every keystroke made
// typing in the search box block the UI for hundreds of milliseconds.
func TestLibrary_SearchDoesNotRescanDownloadStatus(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, root := newLibraryFixture(t, 3)

	require.True(t, lt.state.downloaded(1), "startup must work out the download status")
	require.True(t, lt.state.downloaded(3))

	// Removing the files makes any filesystem rescan observable.
	require.NoError(t, os.RemoveAll(root))
	lt.searchEntry.SetText("Game 1")

	require.True(t, lt.state.downloaded(1), "searching must not rescan the filesystem")
	require.True(t, lt.state.downloaded(3), "a filtered-out game must keep its status")
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

// hangingStoreStub is a store that holds every answer until the test is over,
// then says not found. Nothing can land in the details pane while the test
// runs, and what lands after it is delivered to widgets nothing reads anymore.
func hangingStoreStub(t *testing.T) string {
	t.Helper()
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-gate
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	// Registered after Close, so the gate opens first and Close can finish.
	t.Cleanup(func() { close(gate) })
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
		test.Tap(iconButtonWithTip(lt.content, tipRefresh))
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

// A search that matched nothing left a blank list, which read as a library
// that had not loaded rather than as an answer.
func TestLibrary_SearchWithNoMatchesSaysSo(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)

		lt.searchEntry.SetText("nothing named this")
		require.Contains(t, labelTexts(lt.content), "No games match")

		test.Tap(buttonWithLabel(lt.content, "Clear Search"))
		require.NotContains(t, labelTexts(lt.content), "No games match")
		require.Len(t, lt.listed(), 2)
	})
}

// The search runs when the typing rests, not between two keystrokes.
func TestDebounced_RunsOnceTheChangesRest(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	var runs atomic.Int64
	handler := debounced(30*time.Millisecond, func() { runs.Add(1) })

	handler("g")
	handler("ga")
	handler("gam")
	require.Zero(t, runs.Load(), "a keystroke alone must not run the search")

	require.Eventually(t, func() bool { return runs.Load() == 1 },
		5*time.Second, 10*time.Millisecond)
	require.Never(t, func() bool { return runs.Load() > 1 },
		200*time.Millisecond, 20*time.Millisecond)
}

// Enter on the focused list or grid downloads what is selected: the last step
// of a flow the arrow keys and space already carry.
func TestLibrary_EnterDownloadsTheSelectedGame(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)
	started := 0
	lt.pane.form.download = func() { started++ }

	grid := widgetsOfType[*activatableGrid](lt.content)[0]
	grid.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	require.Equal(t, 1, started, "Enter on the grid starts the download")

	iconButtonWithTip(lt.content, tipShowList).OnTapped()
	list := widgetsOfType[*activatableList](lt.content)[0]
	list.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnter})
	require.Equal(t, 2, started, "and on the list, either Enter key")
}

// Escape is the way out of a filter: it clears the box and brings the whole
// catalogue back.
func TestSearchBox_EscapeClearsTheSearch(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 3)
	lt.searchEntry.SetText("Game 1")
	require.Len(t, lt.listed(), 1)

	lt.searchEntry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	require.Empty(t, lt.searchEntry.Text)
	require.Len(t, lt.listed(), 3, "clearing the search brings everything back")
}

// A zero delay runs at once, on the caller: the tests type and look in the
// same breath.
func TestDebounced_ZeroDelayRunsAtOnce(t *testing.T) {
	var runs atomic.Int64
	handler := debounced(0, func() { runs.Add(1) })

	handler("g")
	require.Equal(t, int64(1), runs.Load())
}

// Working out what a download changed reads the filesystem, which is not work
// for whichever thread the change landed on.
func TestLibrary_DownloadChangesGoThroughTheStatusWorker(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	var calls atomic.Int64
	original := statusWorker
	statusWorker = func(work func() gameStatuses, apply func(gameStatuses)) {
		calls.Add(1)
		apply(work())
	}
	t.Cleanup(func() { statusWorker = original })

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 1)
		before := calls.Load()

		task := &DownloadTask{
			ID: 1, InstanceID: time.Now(), Title: "Game 1",
			Status: binding.NewString(), Details: binding.NewString(),
			Progress: binding.NewFloat(), FileStatus: binding.NewString(),
		}
		require.NoError(t, lt.dm.AddTask(task))

		require.Greater(t, calls.Load(), before,
			"the scan for what the download changed must go through the worker")
	})
}

// The pane said nothing while GOG was being asked, so a description still on
// its way and a game with no description looked the same.
func TestDetailsPane_SaysTheStoreIsBeingAsked(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	// The fixture's store holds its answer, so the note stays while the test
	// looks at it.
	lt, _ := newLibraryFixture(t, 1)

	offMain(t, func() {
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1"}))
		require.True(t, lt.pane.storeStatus.Visible())
		require.Equal(t, "Fetching store details...", lt.pane.storeStatus.Text)

		require.NoError(t, lt.selected.Set(nil))
		require.False(t, lt.pane.storeStatus.Visible(),
			"clicking away takes the note with it")
	})
}

// newLibraryFixtureShown is a library that is what its window is showing.
func newLibraryFixtureShown(t *testing.T, games int) (*libraryTab, fyne.Window) {
	t.Helper()
	lt, _, win := newLibraryFixtureInWindow(t, games)
	win.SetContent(lt.content)
	return lt, win
}
