package gui

import (
	"testing"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
)

// The download goroutine updates the task state while the UI reads it, so the
// state must be safe for concurrent access. Run with -race.
func TestDownloadTaskState_ConcurrentAccess(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	task := &DownloadTask{
		ID:         1,
		Status:     binding.NewString(),
		Details:    binding.NewString(),
		Progress:   binding.NewFloat(),
		FileStatus: binding.NewString(),
	}
	task.SetState(StatePreparing)

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	require.NoError(t, dm.AddTask(task))

	updater := &progressUpdater{
		task:         task,
		fileBytes:    make(map[string]int64),
		fileProgress: make(map[string]struct{ current, total int64 }),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 300; i++ {
			_, _ = updater.Write([]byte(
				`{"type":"start","overall_total":1000}` + "\n" +
					`{"type":"file_progress","file":"a","current":1,"total":1000}` + "\n"))
			task.SetState(StatePreparing)
		}
	}()

	for i := 0; i < 300; i++ {
		_ = dm.activeCount()
	}
	<-done

	// A file_progress update moves a preparing task to downloading.
	task.SetState(StatePreparing)
	_, _ = updater.Write([]byte(`{"type":"file_progress","file":"a","current":2,"total":1000}` + "\n"))
	require.Equal(t, StateDownloading, task.State())
}

// Every byte has landed but the call has not returned: the pause between the
// last byte and completion is files being renamed into place, and it is said.
func TestProgressUpdater_SaysWhenItIsFinishingUp(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	task := &DownloadTask{
		ID:         1,
		Status:     binding.NewString(),
		Details:    binding.NewString(),
		Progress:   binding.NewFloat(),
		FileStatus: binding.NewString(),
	}
	task.SetState(StatePreparing)

	updater := &progressUpdater{
		task:         task,
		fileBytes:    make(map[string]int64),
		fileProgress: make(map[string]struct{ current, total int64 }),
	}

	_, err := updater.Write([]byte(
		`{"type":"start","overall_total":100}` + "\n" +
			`{"type":"file_progress","file":"setup.exe","current":40,"total":100}`))
	require.NoError(t, err)
	status, _ := task.Status.Get()
	require.Equal(t, "Downloading files...", status, "mid-transfer is still downloading")

	_, err = updater.Write([]byte(`{"type":"file_progress","file":"setup.exe","current":100,"total":100}`))
	require.NoError(t, err)
	status, _ = task.Status.Get()
	require.Equal(t, "Finishing up...", status)
}
