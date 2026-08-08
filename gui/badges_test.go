package gui

import (
	"testing"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// runningDownload is a download in flight for a game, registered with a
// manager of its own.
func runningDownload(t *testing.T, gameID int) (*DownloadManager, *DownloadTask) {
	t.Helper()
	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	task := &DownloadTask{
		ID: gameID, Title: "One",
		Status: binding.NewString(), Details: binding.NewString(),
		Progress: binding.NewFloat(), FileStatus: binding.NewString(),
	}
	task.SetState(StateDownloading)
	require.NoError(t, dm.AddTask(task))
	return dm, task
}

// While its files are on their way, a game in the list is its progress; the
// finished marks come back once it is done.
func TestGameRow_ShowsTheDownloadHappeningNow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	state := newLibraryState()
	state.statuses[1] = updateStatus{Downloaded: true}

	dm, task := runningDownload(t, 1)

	offMain(t, func() {
		row := newGameRow().(*gameRow)
		bindGameRow(row, db.Game{ID: 1, Title: "One"}, rowBinding{sel: newGameSelection(), dm: dm, state: state})

		require.True(t, row.badges.progress.Visible(), "a running download is its progress")
		require.False(t, row.badges.downloaded.Visible(), "the tick waits for it to finish")

		require.NoError(t, task.Progress.Set(0.5))
		require.Equal(t, 0.5, row.badges.progress.Value, "the bar follows the download")

		task.SetState(StateCompleted)
		bindGameRow(row, db.Game{ID: 1, Title: "One"}, rowBinding{sel: newGameSelection(), dm: dm, state: state})
		require.False(t, row.badges.progress.Visible())
		require.True(t, row.badges.downloaded.Visible())
	})
}

// The grid cell carries the same progress, drawn across the cell.
func TestGameCell_ShowsTheDownloadHappeningNow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm, _ := runningDownload(t, 1)

	offMain(t, func() {
		cell := newGameCell().(*gameCell)
		bindGameCell(cell, db.Game{ID: 1, Title: "One"}, rowBinding{sel: newGameSelection(), dm: dm})
		require.True(t, cell.badges.progress.Visible())

		bindGameCell(cell, db.Game{ID: 2, Title: "Two"}, rowBinding{sel: newGameSelection(), dm: dm})
		require.False(t, cell.badges.progress.Visible(), "the neighbour is not downloading anything")
	})
}

// The cell says what a game runs on, under its title.
func TestGameCell_NamesThePlatformsUnderTheTitle(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	cell := newGameCell().(*gameCell)
	bindGameCell(cell, db.Game{ID: 1, Title: "Rich Game", Data: richGameData},
		rowBinding{sel: newGameSelection()})

	require.True(t, cell.platforms.Visible())
	require.Equal(t, "Windows · macOS · Linux", cell.platforms.Text)

	// A game whose data offers no downloads says so, rather than leaving a
	// blank line that reads as missing data.
	bindGameCell(cell, db.Game{ID: 2, Title: "Bare Game"}, rowBinding{sel: newGameSelection()})
	require.True(t, cell.platforms.Visible())
	require.Equal(t, "No downloads", cell.platforms.Text)
}

// The pane's title is its headline, so it is set in headline type.
func TestDetailsPane_TitleIsAHeadline(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 1)

	require.Equal(t, theme.SizeNameSubHeadingText, lt.pane.title.SizeName)
}

// Wiping the download history is asked about first: it is also the record of
// where everything went.
func TestDownloadsTab_ClearAllAsksFirst(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	task := &DownloadTask{
		ID: 1, Title: "One",
		Status: binding.NewString(), Details: binding.NewString(),
		Progress: binding.NewFloat(), FileStatus: binding.NewString(),
	}
	task.SetState(StateCompleted)
	require.NoError(t, dm.AddTask(task))

	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	offMain(t, func() {
		tab := DownloadsTabUI(win, dm)
		win.SetContent(tab)

		test.Tap(buttonWithLabel(tab, "Clear All Finished"))
		require.Len(t, dm.tasksSnapshot(), 1, "nothing goes before the question is answered")

		overlay := win.Canvas().Overlays().Top()
		require.NotNil(t, overlay, "the button asks first")
		test.Tap(buttonWithLabel(overlay, "Yes"))
		require.Empty(t, dm.tasksSnapshot())
	})
}
