package gui

import (
	"testing"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// A finished download has no transfers to list and no speed to report, so it
// should not reserve the room an active one needs.
func TestDownloadRowHeights_CompactIsShorterThanFull(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	compact, full := downloadRowHeights()
	t.Logf("card heights: compact %.0f, full %.0f", compact, full)

	require.Less(t, compact, full)
	require.Less(t, compact, full-monospaceLineHeight()*float32(fileStatusLines),
		"a finished card must drop the whole file list, not trim a line")
}

func TestSetRowExpanded_HidesWhatAFinishedDownloadDoesNotNeed(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	_, full := downloadRowHeights()

	row := newDownloadRow()
	setRowExpanded(row, false)
	require.Less(t, row.MinSize().Height, full,
		"hiding the file list must actually shrink the card")

	setRowExpanded(row, true)
	require.Equal(t, full, row.MinSize().Height)
}

// Which downloads need the full card is a decision worth pinning; the list
// itself keeps its per-row heights private.
func TestRowExpanded_OnlyRunningDownloadsNeedTheFullCard(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	for state, expanded := range map[int]bool{
		StatePreparing:   true,
		StateDownloading: true,
		StateCompleted:   false,
		StateCancelled:   false,
		StateError:       false,
	} {
		task := progressingTask(t, state, 0, 0, 0)
		require.Equal(t, expanded, rowExpanded(task), "state %d", state)
	}
}

func TestDownloadsTabUI_ListsEveryDownload(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	require.NoError(t, dm.AddTask(progressingTask(t, StateDownloading, 1<<20, 4<<20, 1<<10)))
	require.NoError(t, dm.AddTask(progressingTask(t, StateCancelled, 1<<20, 4<<20, 0)))

	tab := DownloadsTabUI(test.NewWindow(nil), dm)

	lists := widgetsOfType[*widget.List](tab)
	require.Len(t, lists, 1)
	require.Equal(t, 2, lists[0].Length())
}

// Rendering a row runs the list's update callback, which is where a card that
// no longer matches its template blows up.
func TestDownloadsTabUI_BindsEveryKindOfRowWithoutPanicking(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	for _, state := range []int{StatePreparing, StateDownloading, StateCompleted, StateCancelled, StateError} {
		task := progressingTask(t, state, 1<<20, 4<<20, 1<<10)
		task.Title = "Some Game"
		require.NoError(t, dm.AddTask(task))
	}

	tab := DownloadsTabUI(test.NewWindow(nil), dm)
	lists := widgetsOfType[*widget.List](tab)
	require.Len(t, lists, 1)
	list := lists[0]

	require.Equal(t, 5, list.Length())
	for id := 0; id < list.Length(); id++ {
		row := list.CreateItem()
		require.NotPanics(t, func() { list.UpdateItem(id, row) }, "row %d", id)
	}
}

// A retryable download offers Retry; one restored from history cannot.
func TestDownloadsTabUI_ShowsRetryOnlyWhenItCanRepeatTheDownload(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	retryable := finishedTask(t, StateError, queuedDownload{
		game: db.Game{ID: 1, Title: "Retryable", Data: "{}"}, downloadPath: t.TempDir(),
	})
	restored := finishedTask(t, StateError, queuedDownload{})
	restored.Title = "Restored"
	require.NoError(t, dm.AddTask(retryable))
	require.NoError(t, dm.AddTask(restored))

	list := widgetsOfType[*widget.List](DownloadsTabUI(test.NewWindow(nil), dm))[0]

	// Which row is which is up to the order the tab shows them in, so the rows
	// are read by the download they are for.
	offered := map[string]string{}
	row := list.CreateItem()
	for id := 0; id < list.Length(); id++ {
		list.UpdateItem(id, row)
		offered[row.(*downloadRow).title.Text] = row.(*downloadRow).actionBtn.Text
	}

	require.Equal(t, "Retry", offered["Retryable"])
	require.Equal(t, "Error", offered["Restored"])
}
