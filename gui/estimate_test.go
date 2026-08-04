package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func sizedGame(id int, title string) db.Game {
	return db.Game{ID: id, Title: title, Data: sizedGameData}
}

func estimationApp(t *testing.T) {
	t.Helper()
	prefs := test.NewApp().Preferences()
	prefs.SetString("downloadForm.language", "en")
	prefs.SetString("downloadForm.platform", "windows")
	prefs.SetBool("downloadForm.extras", false)
	prefs.SetBool("downloadForm.dlcs", false)
	sizeCache = make(map[sizeCacheKey]int64)
}

// Estimating uses the settings already on the download form, so what you are
// told is what you would get.
func TestEstimateSelection(t *testing.T) {
	estimationApp(t)

	estimates, total := estimateSelection([]db.Game{sizedGame(1, "One"), sizedGame(2, "Two")})

	require.Len(t, estimates, 2)
	require.Equal(t, "One", estimates[0].Title)
	require.Equal(t, int64(1024*1024*1024), estimates[0].Bytes)
	require.Equal(t, int64(2*1024*1024*1024), total)
}

func TestEstimateSelection_FollowsThePlatformSetting(t *testing.T) {
	estimationApp(t)

	_, windows := estimateSelection([]db.Game{sizedGame(1, "One")})

	test.NewApp().Preferences().SetString("downloadForm.platform", "linux")
	sizeCache = make(map[sizeCacheKey]int64)
	_, linux := estimateSelection([]db.Game{sizedGame(1, "One")})

	require.NotEqual(t, windows, linux)
}

// A game whose stored data cannot be read is reported as unknown rather than
// skipped, so the list still adds up to what was asked for.
func TestEstimateSelection_KeepsUnreadableGamesInTheList(t *testing.T) {
	estimationApp(t)

	estimates, total := estimateSelection([]db.Game{
		sizedGame(1, "Fine"),
		{ID: 2, Title: "Broken", Data: "{not json"},
	})

	require.Len(t, estimates, 2)
	require.Equal(t, "Broken", estimates[1].Title)
	require.Zero(t, estimates[1].Bytes)
	require.Equal(t, int64(1024*1024*1024), total)
}

func TestSizeEstimateCSV(t *testing.T) {
	csv := sizeEstimateCSV([]gameSizeEstimate{
		{Title: `Sam & Max: "Hit the Road"`, Bytes: 1024 * 1024},
		{Title: "Other", Bytes: 0},
	}, 1024*1024)

	lines := strings.Split(strings.TrimSpace(csv), "\n")
	require.Len(t, lines, 4, "a header, both games and a total")
	require.Contains(t, lines[0], "Game")
	require.Contains(t, csv, `"Sam & Max: ""Hit the Road"""`, "quotes are escaped")
	require.Contains(t, lines[len(lines)-1], "Total")
}

// The estimate is an action on the library selection, not a tab of its own.
func TestLibraryTab_OffersToEstimateTheSelection(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)
	require.NotNil(t, buttonWithLabel(lt.content, "Estimate Size"))
}

func TestFileTabUI_NoLongerCarriesItsOwnGameList(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	defer win.Close()

	tab := FileTabUI(win)

	for _, label := range []string{"Estimate Selected Games", "Estimate All Filtered Games", "Select All Shown"} {
		require.Nil(t, buttonWithLabel(tab, label), "%q belongs to the library now", label)
	}
	require.NotNil(t, buttonWithLabel(tab, "Generate File Hashes"))
}
