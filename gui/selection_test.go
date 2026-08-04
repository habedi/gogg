package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func TestGameSelection(t *testing.T) {
	sel := newGameSelection()
	require.Zero(t, sel.count())

	sel.set(1, true)
	sel.set(2, true)
	require.True(t, sel.has(1))
	require.False(t, sel.has(3))
	require.Equal(t, 2, sel.count())

	sel.set(1, false)
	require.False(t, sel.has(1))
	require.Equal(t, 1, sel.count())

	sel.clear()
	require.Zero(t, sel.count())
}

// Selecting everything shown must not touch games hidden by a filter, and the
// selection has to survive the list being refiltered.
func TestGameSelection_SelectAllAppliesToWhatIsShown(t *testing.T) {
	sel := newGameSelection()
	all := []db.Game{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}, {ID: 3, Title: "C"}}
	shown := all[:2]

	sel.selectAll(shown)
	require.Equal(t, 2, sel.count())
	require.False(t, sel.has(3))

	// The selection is keyed by game, not by position in the list.
	require.Equal(t, []db.Game{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}}, sel.gamesIn(all))
}

// A game that has left the catalogue must drop out of the selection quietly.
func TestGameSelection_GamesInIgnoresUnknownIDs(t *testing.T) {
	sel := newGameSelection()
	sel.set(1, true)
	sel.set(99, true)

	require.Equal(t, []db.Game{{ID: 1, Title: "A"}}, sel.gamesIn([]db.Game{{ID: 1, Title: "A"}}))
}

// List rows are recycled. Pointing a checkbox at a new item must not fire the
// handler still attached from the item it showed before.
func TestBindCheck_DoesNotFireThePreviousHandler(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	var fired []string
	check := widget.NewCheck("", nil)

	bindCheck(check, true, func(bool) { fired = append(fired, "first") })
	bindCheck(check, false, func(bool) { fired = append(fired, "second") })
	require.Empty(t, fired, "rebinding must not fire either handler")

	check.SetChecked(true)
	require.Equal(t, []string{"second"}, fired, "only the current handler may fire")
}

func TestBindGameRow_RecyclingDoesNotLeakSelection(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	sel := newGameSelection()
	sel.set(1, true)

	row := newGameRow()
	bindGameRow(row, db.Game{ID: 1, Title: "One"}, sel, nil)
	bindGameRow(row, db.Game{ID: 2, Title: "Two"}, sel, nil)

	require.True(t, sel.has(1), "recycling a row must not deselect the game it used to show")
	require.False(t, sel.has(2))
}

func TestBindGameRow_TicksTheBoxForSelectedGames(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	sel := newGameSelection()
	sel.set(7, true)

	row := newGameRow()
	bindGameRow(row, db.Game{ID: 7, Title: "Seven"}, sel, nil)

	require.True(t, row.(*gameRow).check.Checked)

	bindGameRow(row, db.Game{ID: 8, Title: "Eight"}, sel, nil)
	require.False(t, row.(*gameRow).check.Checked)
}

// A title that is given no width renders as "..." for every game in the list.
func TestGameRow_TitleGetsTheRemainingWidth(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	row := newGameRow()
	bindGameRow(row, db.Game{ID: 1, Title: "A Game With A Reasonably Long Title"}, newGameSelection(), nil)

	const rowWidth = 400
	test.WidgetRenderer(row.(*gameRow)) // force the renderer, as the list does
	row.(*gameRow).Resize(fyne.NewSize(rowWidth, 40))

	title := row.(*gameRow).title
	require.Greater(t, title.Size().Width, float32(rowWidth/2),
		"the title must take the width left by the leading controls")
}

// queueingFixture returns a manager that is already at its concurrency limit,
// so QueueOrStart queues instead of starting anything.
func queueingFixture(t *testing.T) *DownloadManager {
	t.Helper()
	fyne.CurrentApp().Preferences().SetInt("download.maxConcurrent", 1)

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	busy := &DownloadTask{
		ID: 999, Title: "Busy", Status: binding.NewString(),
		Details: binding.NewString(), Progress: binding.NewFloat(), FileStatus: binding.NewString(),
	}
	busy.SetState(StateDownloading)
	require.NoError(t, dm.AddTask(busy))
	return dm
}

func TestQueueDownloads_QueuesEveryGame(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	dm := queueingFixture(t)
	dir := t.TempDir()

	games := []db.Game{{ID: 1, Title: "One", Data: "{}"}, {ID: 2, Title: "Two", Data: "{}"}, {ID: 3, Title: "Three", Data: "{}"}}
	result := queueDownloads(dm, games, func(g db.Game) queuedDownload {
		return queuedDownload{game: g, downloadPath: dir, language: "English", platformName: "windows", numThreads: 1}
	})

	require.Equal(t, 3, result.Queued)
	require.Empty(t, result.Skipped)
	require.Empty(t, result.Failed)

	dm.mu.RLock()
	defer dm.mu.RUnlock()
	require.Len(t, dm.queue, 3)
}

// One game that is already on its way must not stop the rest of the batch.
func TestQueueDownloads_SkipsGamesAlreadyInProgress(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	dm := queueingFixture(t)
	dir := t.TempDir()

	build := func(g db.Game) queuedDownload {
		return queuedDownload{game: g, downloadPath: dir, language: "English", platformName: "windows", numThreads: 1}
	}
	games := []db.Game{{ID: 1, Title: "One", Data: "{}"}, {ID: 2, Title: "Two", Data: "{}"}}

	require.Equal(t, 2, queueDownloads(dm, games, build).Queued)

	// Queueing the same selection again finds both already waiting.
	result := queueDownloads(dm, games, build)
	require.Zero(t, result.Queued)
	require.Equal(t, []string{"One", "Two"}, result.Skipped)
	require.Empty(t, result.Failed)
}

func TestQueueDownloads_ReportsGamesThatCannotStart(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	fyne.CurrentApp().Preferences().SetInt("download.maxConcurrent", 2)
	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	dir := t.TempDir()

	games := []db.Game{{ID: 1, Title: "Broken One", Data: "{not json"}, {ID: 2, Title: "Broken Two", Data: "{not json"}}
	result := queueDownloads(dm, games, func(g db.Game) queuedDownload {
		return queuedDownload{game: g, downloadPath: dir, language: "English", platformName: "windows", numThreads: 1}
	})

	require.Zero(t, result.Queued)
	require.Equal(t, []string{"Broken One", "Broken Two"}, result.Failed,
		"a game that cannot start must be reported, and the batch must carry on")
}

func TestBatchResult_Summary(t *testing.T) {
	require.Contains(t, batchResult{Queued: 1}.summary(), "Queued 1 game")
	require.Contains(t, batchResult{Queued: 3}.summary(), "Queued 3 games")

	summary := batchResult{Queued: 1, Skipped: []string{"A"}, Failed: []string{"B"}}.summary()
	require.Contains(t, summary, "already in progress")
	require.Contains(t, summary, "A")
	require.Contains(t, summary, "could not be started")
	require.Contains(t, summary, "B")
}

// Select All Shown has to reach the whole chain: the selection, the count and
// the download button.
func TestLibraryTab_SelectAllShownSelectsTheListedGames(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 3)

	selectAll := buttonWithLabel(lt.content, "Select All Shown")
	require.NotNil(t, selectAll, "the library must offer Select All Shown")
	download := buttonWithLabel(lt.content, "Download Game")
	require.NotNil(t, download, "the download button starts out in single-game mode")

	selectAll.OnTapped()
	require.Equal(t, "Download Selected (3)", download.Text)

	clear := buttonWithLabel(lt.content, "Clear Selection")
	require.NotNil(t, clear)
	clear.OnTapped()
	require.Equal(t, "Download Game", download.Text)
}
