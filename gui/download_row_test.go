package gui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func monospaceLineHeight() float32 {
	probe := widget.NewLabel("Ag")
	probe.TextStyle = fyne.TextStyle{Monospace: true}
	return probe.MinSize().Height
}

// widget.List gives every row the height of its template, so a card can never
// grow to fit its file list: the template has to reserve the room, or the last
// line is clipped.
func TestDownloadRow_ReservesRoomForTheFileList(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	row := newDownloadRow().(*downloadRow)

	needed := monospaceLineHeight() * float32(fileStatusLines+1) // files plus the summary line
	require.GreaterOrEqual(t, row.fileScroll.MinSize().Height, needed,
		"the card must show every line the file list can produce")
}

func fileProgressUpdater(t *testing.T, files int) (*progressUpdater, *DownloadTask) {
	t.Helper()
	task := &DownloadTask{
		Status: binding.NewString(), Details: binding.NewString(),
		Progress: binding.NewFloat(), FileStatus: binding.NewString(),
	}
	updater := &progressUpdater{
		task:         task,
		fileBytes:    make(map[string]int64),
		fileProgress: make(map[string]struct{ current, total int64 }),
	}
	for i := 0; i < files; i++ {
		updater.fileProgress[fmt.Sprintf("setup_game-%d.bin", i)] =
			struct{ current, total int64 }{current: 1 << 20, total: 4 << 20}
	}
	return updater, task
}

// Downloads run several files at once, and the card lists them.
func TestUpdateFileStatusText_ListsEveryFileItCan(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	updater, task := fileProgressUpdater(t, fileStatusLines)
	updater.updateFileStatusText()

	text, err := task.FileStatus.Get()
	require.NoError(t, err)
	require.Len(t, strings.Split(text, "\n"), fileStatusLines,
		"no summary line is needed when every file fits")
	require.NotContains(t, text, "more files")
}

func TestUpdateFileStatusText_SummarisesTheRest(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	updater, task := fileProgressUpdater(t, fileStatusLines+2)
	updater.updateFileStatusText()

	text, err := task.FileStatus.Get()
	require.NoError(t, err)

	lines := strings.Split(text, "\n")
	require.Len(t, lines, fileStatusLines+1, "the listed files plus one summary line")
	require.Contains(t, lines[len(lines)-1], "2 more files")
}

func TestUpdateFileStatusText_EmptyWhenNothingIsInFlight(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	updater, task := fileProgressUpdater(t, 0)
	updater.updateFileStatusText()

	text, err := task.FileStatus.Get()
	require.NoError(t, err)
	require.Empty(t, text)
}

// The downloads still running are what the tab is for, so they come first. The
// finished ones follow with the most recent at the top, rather than the whole
// history sitting above whatever is happening now.
func TestOrderedTasks_PutsWhatIsRunningFirst(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	start := time.Now()
	task := func(title string, state int, age time.Duration) *DownloadTask {
		task := &DownloadTask{Title: title, InstanceID: start.Add(-age), Status: binding.NewString()}
		task.SetState(state)
		return task
	}
	oldFinished := task("old finished", StateCompleted, 3*time.Hour)
	newFinished := task("new finished", StateError, time.Hour)
	running := task("running", StateDownloading, 2*time.Hour)
	waiting := task("waiting", StatePreparing, time.Minute)

	ordered := orderedTasks([]*DownloadTask{oldFinished, newFinished, running, waiting})

	var titles []string
	for _, task := range ordered {
		titles = append(titles, task.Title)
	}
	require.Equal(t, []string{"running", "waiting", "new finished", "old finished"}, titles)
}

// An empty Downloads tab has to say it is empty rather than show a blank page.
func TestDownloadsTab_SaysWhenThereIsNothingToShow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		dm := &DownloadManager{Tasks: binding.NewUntypedList()}
		ui := DownloadsTabUI(test.NewWindow(nil), dm)
		require.Contains(t, labelTexts(ui), "No downloads yet")

		queued := &DownloadTask{
			ID: 1, Title: "One", Status: binding.NewString(), Details: binding.NewString(),
			Progress: binding.NewFloat(), FileStatus: binding.NewString(),
		}
		require.NoError(t, dm.Tasks.Append(queued))

		require.NotContains(t, labelTexts(ui), "No downloads yet",
			"the list takes over as soon as there is something in it")
	})
}

// The history is what gogg remembers between runs, and it only ever grew.
func TestPersistHistory_KeepsTheMostRecentDownloads(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := NewDownloadManager(nil)
	start := time.Now()
	for i := 0; i < historyKept+50; i++ {
		task := &DownloadTask{
			ID: i, Title: fmt.Sprintf("Game %d", i), InstanceID: start.Add(time.Duration(i) * time.Second),
			Status: binding.NewString(), Details: binding.NewString(),
			Progress: binding.NewFloat(), FileStatus: binding.NewString(),
		}
		task.SetState(StateCompleted)
		require.NoError(t, dm.Tasks.Append(task))
	}

	dm.PersistHistory()

	reopened, err := NewDownloadManager(nil).Tasks.Get()
	require.NoError(t, err)
	require.Len(t, reopened, historyKept, "the history has to stop growing at some point")
	require.Equal(t, fmt.Sprintf("Game %d", historyKept+49), reopened[0].(*DownloadTask).Title,
		"and keep the most recent downloads")
}

// The line under a running download and the line above the list say the same
// things, so they have to say them the same way.
func TestTransferSummary_ReadsLikeTheHeaderAboveTheList(t *testing.T) {
	require.Equal(t, "5.0 MiB/s · ETA 2m0s", transferSummary(5<<20, 600<<20))
	require.Equal(t, "5.0 MiB/s", transferSummary(5<<20, 0), "nothing left to say how long it will take")
	require.Empty(t, transferSummary(0, 100<<20), "a download that has not moved says nothing yet")
}

// titleAt is the download the list shows in a given place.
func titleAt(t *testing.T, list *widget.List, id int) string {
	t.Helper()
	row := list.CreateItem()
	list.UpdateItem(widget.ListItemID(id), row)
	return row.(*downloadRow).title.Text
}

// A download that has just finished belongs with the finished ones. Nothing but
// its own state changing says so, and the tab was only ever told when downloads
// were added or removed.
func TestDownloadsTab_ReordersWhenADownloadFinishes(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		dm := &DownloadManager{Tasks: binding.NewUntypedList()}
		ui := DownloadsTabUI(test.NewWindow(nil), dm)

		start := time.Now()
		running := &DownloadTask{
			Title: "running", InstanceID: start.Add(-time.Hour), Status: binding.NewString(),
			Details: binding.NewString(), Progress: binding.NewFloat(), FileStatus: binding.NewString(),
		}
		running.SetState(StateDownloading)
		finished := &DownloadTask{
			Title: "finished", InstanceID: start, Status: binding.NewString(),
			Details: binding.NewString(), Progress: binding.NewFloat(), FileStatus: binding.NewString(),
		}
		finished.SetState(StateCompleted)

		// Adding a download tells the tab, which asks the manager what it holds:
		// that must not deadlock.
		require.NoError(t, dm.AddTask(running))
		require.NoError(t, dm.AddTask(finished))

		// The list takes the place of the empty state once there is something
		// in it.
		list := widgetsOfType[*widget.List](ui)[0]
		require.Equal(t, "running", titleAt(t, list, 0), "what is running comes first")

		running.SetState(StateCompleted)

		require.Equal(t, "finished", titleAt(t, list, 0),
			"once it is done it takes its place among the finished, newest first")
	})
}

// A download still running when gogg stops comes back as interrupted, with
// its request intact so it can be resumed after a restart.
func TestPersistHistory_InterruptedDownloadSurvivesAndResumes(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := NewDownloadManager(stubAuthService())
	require.NoError(t, dm.Tasks.Set(nil)) // start from an empty history, whatever a prior test left
	running := &DownloadTask{
		ID: 7, Title: "Big Game", InstanceID: time.Now(),
		Status: binding.NewString(), Details: binding.NewString(),
		Progress: binding.NewFloat(), FileStatus: binding.NewString(),
		DownloadPath: "/games/big-game",
		request: queuedDownload{
			authService:  stubAuthService(),
			game:         db.Game{ID: 7, Title: "Big Game", Data: "{}"},
			downloadPath: "/games", language: "en", platformName: "windows",
			numThreads: 4, connections: 2,
		},
	}
	running.SetState(StateDownloading)
	require.NoError(t, dm.Tasks.Append(running))

	dm.PersistHistory()

	// A fresh manager, as after a restart, reads the history back.
	reopened := NewDownloadManager(stubAuthService())
	tasks, err := reopened.Tasks.Get()
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	restored := tasks[0].(*DownloadTask)
	require.Equal(t, StateInterrupted, restored.State(),
		"a download that was running is interrupted, not lost")
	require.True(t, restored.canRetry(), "and it carries the request a resume needs")
	require.Equal(t, 7, restored.request.game.ID)
	require.Equal(t, 2, restored.request.connections, "every download option survives the round trip")
}

// A completed or failed download keeps its own state across a restart, not
// the interrupted one.
func TestPersistHistory_FinishedStatesAreUnchanged(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := NewDownloadManager(stubAuthService())
	require.NoError(t, dm.Tasks.Set(nil)) // start from an empty history
	for _, tc := range []struct {
		id    int
		state int
	}{{1, StateCompleted}, {2, StateError}, {3, StatePaused}} {
		task := &DownloadTask{
			ID: tc.id, Title: fmt.Sprintf("Game %d", tc.id), InstanceID: time.Now().Add(time.Duration(tc.id) * time.Second),
			Status: binding.NewString(), Details: binding.NewString(),
			Progress: binding.NewFloat(), FileStatus: binding.NewString(),
			request: queuedDownload{game: db.Game{ID: tc.id, Data: "{}"}},
		}
		task.SetState(tc.state)
		require.NoError(t, dm.Tasks.Append(task))
	}
	dm.PersistHistory()

	tasks, _ := NewDownloadManager(stubAuthService()).Tasks.Get()
	byID := map[int]int{}
	for _, raw := range tasks {
		task := raw.(*DownloadTask)
		byID[task.ID] = task.State()
	}
	require.Equal(t, StateCompleted, byID[1])
	require.Equal(t, StateError, byID[2])
	require.Equal(t, StatePaused, byID[3], "a paused download stays paused, not interrupted")
}

// An interrupted download's row offers Resume: its bytes are on disk, so
// carrying on is what the button does.
func TestDownloadsTab_InterruptedRowOffersResume(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		dm := &DownloadManager{Tasks: binding.NewUntypedList()}
		ui := DownloadsTabUI(test.NewWindow(nil), dm)

		interrupted := &DownloadTask{
			ID: 7, Title: "Big Game", InstanceID: time.Now(),
			Status: binding.NewString(), Details: binding.NewString(),
			Progress: binding.NewFloat(), FileStatus: binding.NewString(),
			request: queuedDownload{game: db.Game{ID: 7, Data: "{}"}},
		}
		interrupted.SetState(StateInterrupted)
		require.NoError(t, dm.AddTask(interrupted))

		list := widgetsOfType[*widget.List](ui)[0]
		row := list.CreateItem()
		list.UpdateItem(0, row)
		require.Equal(t, "Resume", row.(*downloadRow).actionBtn.Text,
			"an interrupted download carries on rather than restarts")
	})
}
