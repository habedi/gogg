package gui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func finishedTask(t *testing.T, state int, request queuedDownload) *DownloadTask {
	t.Helper()
	task := &DownloadTask{
		ID: request.game.ID, InstanceID: time.Now(), Title: request.game.Title,
		Status: binding.NewString(), Details: binding.NewString(),
		Progress: binding.NewFloat(), FileStatus: binding.NewString(),
		request: request,
	}
	task.SetState(state)
	return task
}

// A download that failed has to be restartable from the Downloads tab, rather
// than sending the user back to the catalogue to set it up again.
func TestRetry_RequeuesAFailedDownload(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	dm := queueingFixture(t) // already at the concurrency limit, so retries queue

	request := queuedDownload{
		authService: stubAuthService(), game: db.Game{ID: 1, Title: "One", Data: "{}"},
		downloadPath: t.TempDir(), language: "English", platformName: "windows", numThreads: 1,
	}
	failed := finishedTask(t, StateError, request)
	require.NoError(t, dm.AddTask(failed))

	require.NoError(t, dm.retry(failed))

	dm.mu.RLock()
	queued := len(dm.queue)
	dm.mu.RUnlock()
	require.Equal(t, 1, queued, "the download must be queued again")

	tasks, err := dm.Tasks.Get()
	require.NoError(t, err)
	for _, raw := range tasks {
		require.NotEqual(t, failed.InstanceID, raw.(*DownloadTask).InstanceID,
			"the entry that was retried must not linger next to its replacement")
	}
}

func TestRetry_WorksForCancelledDownloadsToo(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	dm := queueingFixture(t)

	cancelled := finishedTask(t, StateCancelled, queuedDownload{
		authService: stubAuthService(), game: db.Game{ID: 2, Title: "Two", Data: "{}"},
		downloadPath: t.TempDir(), language: "English", platformName: "windows", numThreads: 1,
	})
	require.NoError(t, dm.AddTask(cancelled))
	require.True(t, cancelled.canRetry())
	require.NoError(t, dm.retry(cancelled))
}

// Tasks restored from the history file carry no request, so there is nothing to
// repeat and the button must not pretend otherwise.
func TestRetry_RefusesTasksWithNothingToRepeat(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	dm := queueingFixture(t)

	restored := finishedTask(t, StateError, queuedDownload{})
	require.NoError(t, dm.AddTask(restored))

	require.False(t, restored.canRetry())
	require.Error(t, dm.retry(restored))

	dm.mu.RLock()
	defer dm.mu.RUnlock()
	require.Empty(t, dm.queue)
}

func TestRetry_RefusesDownloadsThatAreStillRunning(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	running := finishedTask(t, StateDownloading, queuedDownload{
		game: db.Game{ID: 3, Title: "Three", Data: "{}"},
	})
	require.False(t, running.canRetry())
}

// The request is what makes a retry possible, so starting a download has to
// keep it.
func TestExecuteDownload_RemembersTheRequest(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	// This download completes, and the completion sound is not what is under test.
	app.Preferences().SetBool("soundEnabled", false)

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	request := queuedDownload{
		authService: stubAuthService(), game: db.Game{ID: 4, Title: "Four", Data: "{}"},
		downloadPath: t.TempDir(), language: "English", platformName: "windows", numThreads: 1,
	}

	require.NoError(t, executeDownload(dm, request))

	tasks, err := dm.Tasks.Get()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, request.game.ID, tasks[0].(*DownloadTask).request.game.ID)
	require.Equal(t, request.downloadPath, tasks[0].(*DownloadTask).request.downloadPath)
	awaitSettled(t, dm, 4)
}

// A download cancelled while it was still waiting in the queue has to be
// restartable too. The queued entry carried nothing to repeat, so the row
// offered a dead "Cancelled" button where a started download offers Retry.
func TestRetry_RestartsADownloadCancelledInTheQueue(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	dm := queueingFixture(t) // already at the limit, so this download waits

	request := queuedDownload{
		authService: stubAuthService(), game: db.Game{ID: 7, Title: "Seven", Data: "{}"},
		downloadPath: t.TempDir(), language: "English", platformName: "windows", numThreads: 1,
	}
	require.NoError(t, dm.QueueOrStart(request))

	waiting := taskForGame(t, dm, 7)
	status, _ := waiting.Status.Get()
	require.Equal(t, "Queued", status)

	waiting.CancelFunc() // the Cancel button on a queued row
	require.Equal(t, StateCancelled, waiting.State())
	require.True(t, waiting.canRetry(), "a cancelled download has to be restartable, queued or not")

	require.NoError(t, dm.retry(waiting))
	dm.mu.RLock()
	queued := len(dm.queue)
	dm.mu.RUnlock()
	require.Equal(t, 1, queued, "retrying has to put the download back in the queue")
}

// taskForGame is the download the manager is holding for a game.
func taskForGame(t *testing.T, dm *DownloadManager, gameID int) *DownloadTask {
	t.Helper()
	all, err := dm.Tasks.Get()
	require.NoError(t, err)
	for _, raw := range all {
		if task, ok := raw.(*DownloadTask); ok && task.ID == gameID {
			return task
		}
	}
	t.Fatalf("no download for game %d", gameID)
	return nil
}
