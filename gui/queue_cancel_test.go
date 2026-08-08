package gui

import (
	"testing"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func placeholderFor(t *testing.T, dm *DownloadManager, gameID int) *DownloadTask {
	t.Helper()
	tasks, err := dm.Tasks.Get()
	require.NoError(t, err)
	for _, raw := range tasks {
		task := raw.(*DownloadTask)
		if task.ID != gameID {
			continue
		}
		if status, _ := task.Status.Get(); status == "Queued" {
			return task
		}
	}
	t.Fatalf("no queued task found for game %d", gameID)
	return nil
}

// The Downloads tab offers a Cancel button for queued downloads, so it has to
// actually drop them from the queue.
func TestQueueOrStart_CancelRemovesAQueuedDownload(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Preferences().SetInt("download.maxConcurrent", 1)

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}

	// One download is already running, so the next one is queued.
	active := &DownloadTask{
		ID: 1, Title: "Running", Status: binding.NewString(),
		Details: binding.NewString(), Progress: binding.NewFloat(), FileStatus: binding.NewString(),
	}
	active.SetState(StateDownloading)
	require.NoError(t, dm.AddTask(active))

	require.NoError(t, dm.QueueOrStart(queuedDownload{
		game:         db.Game{ID: 2, Title: "Queued Game", Data: "{}"},
		downloadPath: t.TempDir(),
		language:     "English",
		platformName: "windows",
		numThreads:   1,
	}))

	dm.mu.RLock()
	require.Len(t, dm.queue, 1)
	dm.mu.RUnlock()

	queued := placeholderFor(t, dm, 2)
	require.NotNil(t, queued.CancelFunc, "a queued download must be cancellable")

	queued.CancelFunc()

	dm.mu.RLock()
	remaining := len(dm.queue)
	dm.mu.RUnlock()
	require.Zero(t, remaining, "a cancelled download must not start later")
	require.Equal(t, StateCancelled, queued.State())

	// The freed slot must not be handed back to the cancelled download.
	dm.startNextIfAvailable()
	dm.mu.RLock()
	require.Empty(t, dm.queue)
	dm.mu.RUnlock()
}
