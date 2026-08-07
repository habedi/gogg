package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// gameDataWithVersion is a catalogue payload for one Windows installer.
func gameDataWithVersion(title, version string) string {
	return fmt.Sprintf(`{"title":%q,"downloads":[["English",{"windows":[`+
		`{"manualUrl":"/x","name":"setup.exe","version":%q,"size":"1 GB"}]}]],"extras":[],"dlcs":[]}`,
		title, version)
}

func TestGamesWithUpdates(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	updateStatusCache = map[int]updateStatus{
		1: {Downloaded: true, HasUpdate: true, Diff: []string{"CHANGED: windows|setup.exe 1.0 -> 2.0"}},
		2: {Downloaded: true},
		3: {Downloaded: false},
	}
	t.Cleanup(func() { updateStatusCache = make(map[int]updateStatus) })

	all := []db.Game{{ID: 1, Title: "One"}, {ID: 2, Title: "Two"}, {ID: 3, Title: "Three"}}
	require.Equal(t, []db.Game{{ID: 1, Title: "One"}}, gamesWithUpdates(all))
}

// A game downloaded before gogg started writing download_info.json still has to
// be checked for updates: the stored data names languages in full, so the
// preference has to be translated from its code.
func TestComputeUpdateStatus_DetectsUpdatesWithoutDownloadInfo(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	updateStatusCache = make(map[int]updateStatus)
	t.Cleanup(func() { updateStatusCache = make(map[int]updateStatus) })

	root := t.TempDir()
	dir := filepath.Join(root, client.SanitizePath("Some Game"))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	// What was downloaded, with no download_info.json beside it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"),
		[]byte(gameDataWithVersion("Some Game", "1.0")), 0o644))

	prefs := app.Preferences()
	prefs.SetString("lastUsedDownloadPath", root)
	prefs.SetString("downloadForm.language", "en")
	prefs.SetString("downloadForm.platform", "windows")
	prefs.SetBool("downloadForm.scanDirsForDownloads", true)

	// What GOG offers now.
	games := []db.Game{{ID: 1, Title: "Some Game", Data: gameDataWithVersion("Some Game", "2.0")}}
	computeUpdateStatus(&DownloadManager{Tasks: binding.NewUntypedList()}, games)

	require.True(t, isGameDownloadedCached(1))
	hasUpdate, diff := hasGameUpdateCached(1)
	require.True(t, hasUpdate, "a newer installer version has to register as an update")
	require.NotEmpty(t, diff)
}

// newUpdatableLibrary builds a library where one of two games has an update.
func newUpdatableLibrary(t *testing.T) *libraryTab {
	t.Helper()

	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })
	require.NoError(t, db.UpsertTokenRecord(&db.Token{
		AccessToken: "a", RefreshToken: "r", ExpiresAt: "2999-01-01T00:00:00Z",
	}))

	root := t.TempDir()
	for _, game := range []struct {
		id                        int
		title, stored, catalogued string
	}{
		{1, "Old Game", "1.0", "2.0"}, // an update is waiting
		{2, "Current Game", "3.0", "3.0"},
	} {
		require.NoError(t, db.PutInGame(game.id, game.title, gameDataWithVersion(game.title, game.catalogued)))
		dir := filepath.Join(root, client.SanitizePath(game.title))
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"),
			[]byte(gameDataWithVersion(game.title, game.stored)), 0o644))
	}

	prefs := fyne.CurrentApp().Preferences()
	prefs.SetString("lastUsedDownloadPath", root)
	prefs.SetString("downloadForm.language", "en")
	prefs.SetString("downloadForm.platform", "windows")

	updateStatusCache = make(map[int]updateStatus)
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	return LibraryTabUI(win, nil, &DownloadManager{Tasks: binding.NewUntypedList()}, func() {})
}

func TestLibraryTab_OffersToUpdateEverythingThatHasOne(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt := newUpdatableLibrary(t)

	require.NotNil(t, buttonWithLabel(lt.content, "Update All (1)"),
		"the library must offer to download the waiting update")
}

func TestLibraryTab_HidesUpdateAllWhenNothingIsWaiting(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2) // stored data matches the catalogue

	for _, label := range []string{"Update All (1)", "Update All (2)"} {
		require.Nil(t, buttonWithLabel(lt.content, label))
	}
}

// newUndownloadedLibrary builds a library of games that have never been
// fetched, so what a finished download changes is visible.
func newUndownloadedLibrary(t *testing.T, games int) (*libraryTab, *DownloadManager) {
	t.Helper()

	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })
	require.NoError(t, db.UpsertTokenRecord(&db.Token{
		AccessToken: "a", RefreshToken: "r", ExpiresAt: "2999-01-01T00:00:00Z",
	}))
	for id := 1; id <= games; id++ {
		title := fmt.Sprintf("Game %d", id)
		require.NoError(t, db.PutInGame(id, title, gameDataWithVersion(title, "1.0")))
	}

	prefs := fyne.CurrentApp().Preferences()
	prefs.SetString("lastUsedDownloadPath", t.TempDir()) // nothing has been downloaded there
	prefs.SetString("downloadForm.language", "en")
	prefs.SetString("downloadForm.platform", "windows")

	cache := t.TempDir()
	original := cacheRoot
	cacheRoot = func() string { return cache }
	t.Cleanup(func() { cacheRoot = original })
	if os.Getenv("GOGG_API_BASE") == "" {
		t.Setenv("GOGG_API_BASE", storeStub(t, nil))
	}

	updateStatusCache = make(map[int]updateStatus)
	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	return LibraryTabUI(win, nil, dm, func() {}), dm
}

// A download finishing changes what a game is, so the collections beside the
// list and whatever the list is filtered to both have to follow it.
func TestLibraryTab_FinishedDownloadUpdatesTheCollectionsAndTheList(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, dm := newUndownloadedLibrary(t, 2)
		test.Tap(lt.showCollections)

		require.Equal(t, "0", lt.sidebar.buttons["Downloaded"].count.Text)
		lt.searchEntry.SetText("downloaded:no")
		require.Len(t, lt.listed(), 2, "neither game has been fetched yet")

		finished := &DownloadTask{
			ID: 1, InstanceID: time.Now(), Title: "Game 1",
			Status: binding.NewString(), Details: binding.NewString(),
			Progress: binding.NewFloat(), FileStatus: binding.NewString(),
		}
		finished.SetState(StateCompleted)
		require.NoError(t, dm.Tasks.Append(finished))

		require.Equal(t, "1", lt.sidebar.buttons["Downloaded"].count.Text,
			"the collections have to count the finished download")
		require.Equal(t, "1", lt.sidebar.buttons["Not downloaded"].count.Text)
		require.Len(t, lt.listed(), 1,
			"a game that has just been downloaded is no longer one that is not")
	})
}

// The version map identifies a file by a path through platform, DLC and extras.
// That is how gogg tells two files apart, not something to show anyone, so what
// changed is put into words.
func TestDescribeChange_PutsAFileIntoWords(t *testing.T) {
	require.Equal(t, "setup_game.exe (Windows): 1.2.2 → 1.2.3",
		describeChangedFile("windows|setup_game.exe", "1.2.2", "1.2.3"))
	require.Equal(t, "setup_game.dmg (macOS): new, version 1.0",
		describeAddedFile("mac|setup_game.dmg", "1.0"))
	require.Equal(t, "soundtrack (extra): new",
		describeAddedFile("extras|soundtrack", ""))
	require.Equal(t, "setup_expansion.exe (Windows, Expansion): 1.0 → 1.1",
		describeChangedFile("dlc:Expansion|windows|setup_expansion.exe", "1.0", "1.1"))
	require.Equal(t, "artbook.pdf (Expansion, extra): new",
		describeAddedFile("dlc_extras:Expansion|artbook.pdf", ""))
}

// A file that had no version and now has one still reads as a sentence.
func TestDescribeChange_HandlesAMissingVersion(t *testing.T) {
	require.Equal(t, "setup_game.exe (Windows): now version 2.0",
		describeChangedFile("windows|setup_game.exe", "", "2.0"))
}

// What the update dialog lists is what a person can read.
func TestComputeUpdateStatus_DescribesTheChangeInWords(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := t.TempDir()
	dir := filepath.Join(root, client.SanitizePath("Some Game"))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"),
		[]byte(gameDataWithVersion("Some Game", "1.0")), 0o644))

	prefs := app.Preferences()
	prefs.SetString("lastUsedDownloadPath", root)
	prefs.SetString("downloadForm.language", "en")
	prefs.SetString("downloadForm.platform", "windows")
	prefs.SetBool("downloadForm.scanDirsForDownloads", true)
	updateStatusCache = map[int]updateStatus{}

	computeUpdateStatus(&DownloadManager{Tasks: binding.NewUntypedList()},
		[]db.Game{{ID: 1, Title: "Some Game", Data: gameDataWithVersion("Some Game", "2.0")}})

	_, diff := hasGameUpdateCached(1)
	require.Equal(t, []string{"setup.exe (Windows): 1.0 → 2.0"}, diff)
}

// The worker runs on a thread of its own, so it must not touch what the window
// reads: the cache is written where the window is drawn.
func TestStatusesFor_LeavesTheCacheToTheUIThread(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := t.TempDir()
	dir := filepath.Join(root, client.SanitizePath("Some Game"))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"),
		[]byte(gameDataWithVersion("Some Game", "1.0")), 0o644))
	app.Preferences().SetString("lastUsedDownloadPath", root)
	updateStatusCache = map[int]updateStatus{}

	found := statusesFor(&DownloadManager{Tasks: binding.NewUntypedList()},
		[]db.Game{{ID: 1, Title: "Some Game", Data: gameDataWithVersion("Some Game", "2.0")}})

	require.True(t, found[1].HasUpdate, "the worker still has to do the work")
	require.Empty(t, updateStatusCache, "and leave the cache to the thread that draws the window")
}

// heldStatusWork holds the status work so a test can see the library while it
// is waiting for it.
type heldStatusWork struct {
	work  func() gameStatuses
	apply func(gameStatuses)
}

func holdStatusWork(t *testing.T) *heldStatusWork {
	t.Helper()
	held := &heldStatusWork{}
	original := statusWorker
	statusWorker = func(work func() gameStatuses, apply func(gameStatuses)) {
		held.work, held.apply = work, apply
	}
	t.Cleanup(func() { statusWorker = original })
	return held
}

func (h *heldStatusWork) run() { h.apply(h.work()) }

// Finding out what has been downloaded reads the filesystem and parses every
// stored game. The library says it is looking rather than doing that where the
// window is drawn and showing nothing until it is done.
func TestLibrary_SaysWhileItIsCheckingDownloads(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		refreshes := captureRefreshes(t)
		lt, _ := newLibraryFixture(t, 3)

		held := holdStatusWork(t)
		test.Tap(iconButtonWithTip(lt.content, tipRefresh))
		refreshes.finish()

		require.Contains(t, labelTexts(lt.content), "Checking downloads...")
		require.NotNil(t, held.work, "the work has to be handed to the worker, not done here")

		held.run()
		require.NotContains(t, labelTexts(lt.content), "Checking downloads...")
		require.True(t, isGameDownloadedCached(1), "and what it found is what the library shows")
	})
}

// A library that has been replaced, as it is when the user logs in, has to stop
// listening for catalogue changes. The signal belongs to the whole app, so one
// left listening goes on working for a window that is not there any more.
func TestLibraryTab_CloseStopsItFollowingTheCatalogue(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		held := holdStatusWork(t)

		SignalCatalogueUpdated()
		require.Contains(t, labelTexts(lt.content), "Checking downloads...",
			"a library on screen follows the catalogue")
		held.run()

		lt.close()
		SignalCatalogueUpdated()

		require.NotContains(t, labelTexts(lt.content), "Checking downloads...",
			"a library that has been replaced does not")
	})
}
