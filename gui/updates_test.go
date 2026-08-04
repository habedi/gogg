package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

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
