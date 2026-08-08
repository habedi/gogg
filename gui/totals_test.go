package gui

import (
	"testing"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
)

func progressingTask(t *testing.T, state int, downloaded, total, speed int64) *DownloadTask {
	t.Helper()
	task := &DownloadTask{
		Status: binding.NewString(), Details: binding.NewString(),
		Progress: binding.NewFloat(), FileStatus: binding.NewString(),
	}
	task.SetState(state)
	task.SetProgressBytes(downloaded, total, speed)
	return task
}

// The per-download cards say nothing about the whole batch, which is what you
// want to know when twelve games are queued.
func TestDownloadTotals_SumsWhatIsStillRunning(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	tasks := []*DownloadTask{
		progressingTask(t, StateDownloading, 1_000, 4_000, 100),
		progressingTask(t, StateDownloading, 2_000, 6_000, 200),
		progressingTask(t, StateCompleted, 9_000, 9_000, 0), // done, not counted
		progressingTask(t, StateError, 5, 100, 0),           // failed, not counted
	}

	active, downloaded, total, speed := downloadTotals(tasks)
	require.Equal(t, 2, active)
	require.Equal(t, int64(3_000), downloaded)
	require.Equal(t, int64(10_000), total)
	require.Equal(t, int64(300), speed)
}

func TestDownloadTotals_CountsPreparingDownloads(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	active, _, _, _ := downloadTotals([]*DownloadTask{progressingTask(t, StatePreparing, 0, 0, 0)})
	require.Equal(t, 1, active, "a download that has not transferred anything yet is still running")
}

func TestTotalsSummary(t *testing.T) {
	require.Empty(t, totalsSummary(0, 0, 0, 0), "nothing running, nothing to say")

	summary := totalsSummary(3, 2<<30, 10<<30, 5<<20)
	require.Contains(t, summary, "3 downloads")
	require.Contains(t, summary, "2.0 GiB of 10.0 GiB")
	require.Contains(t, summary, "20%")
	require.Contains(t, summary, "5.0 MiB/s")
	require.Contains(t, summary, "ETA")

	require.Contains(t, totalsSummary(1, 100, 200, 0), "1 download")
}

// Sizes are not known until the first response arrives.
func TestTotalsSummary_WithoutAKnownTotal(t *testing.T) {
	summary := totalsSummary(1, 1<<20, 0, 0)
	require.Contains(t, summary, "1.0 MiB")
	require.NotContains(t, summary, "of 0")
	require.NotContains(t, summary, "%")
}

// The header is fed from the same updates that drive each card.
func TestProgressUpdater_KeepsTheTaskTotalsUpToDate(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	task := progressingTask(t, StatePreparing, 0, 0, 0)
	require.NoError(t, dm.AddTask(task))

	updater := &progressUpdater{
		task: task, dm: dm,
		fileBytes:    make(map[string]int64),
		fileProgress: make(map[string]struct{ current, total int64 }),
	}

	_, err := updater.Write([]byte(`{"type":"start","overall_total":8000}` + "\n" +
		`{"type":"file_progress","file":"a","current":2000,"total":4000}` + "\n"))
	require.NoError(t, err)

	downloaded, total, _ := task.ProgressBytes()
	require.Equal(t, int64(2000), downloaded)
	require.Equal(t, int64(8000), total)

	shown, err := dm.totals().Get()
	require.NoError(t, err)
	require.Contains(t, shown, "1 download")
}

// The Downloads tab shows the aggregate above the list.
func TestDownloadsTabUI_ShowsTheAggregate(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	require.NoError(t, dm.AddTask(progressingTask(t, StateDownloading, 1<<20, 4<<20, 1<<10)))

	tab := DownloadsTabUI(test.NewWindow(nil), dm)

	var shown []string
	for _, label := range widgetsOfType[*widget.Label](tab) {
		shown = append(shown, label.Text)
	}
	require.Contains(t, shown, totalsSummary(1, 1<<20, 4<<20, 1<<10))
}

// The headline speaks in every state, not only while something runs: a list
// of finished downloads must not sit under a blank bar.
func TestDownloadsHeadline_SpeaksWhenNothingRuns(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	require.Empty(t, downloadsHeadline(nil), "with no downloads the empty state speaks instead")

	// While something runs, the active summary wins.
	running := downloadsHeadline([]*DownloadTask{
		progressingTask(t, StateDownloading, 1<<20, 4<<20, 1<<10),
		progressingTask(t, StateCompleted, 9, 9, 0),
	})
	require.Contains(t, running, "1 download", "the active summary is preferred")

	// With nothing running, it counts what is finished, paused, and failed.
	resting := downloadsHeadline([]*DownloadTask{
		progressingTask(t, StateCompleted, 9, 9, 0),
		progressingTask(t, StateCompleted, 9, 9, 0),
		progressingTask(t, StatePaused, 1, 9, 0),
		progressingTask(t, StateError, 1, 9, 0),
	})
	require.Equal(t, "2 finished · 1 paused · 1 failed", resting)
}

// A download running in passes counts bytes across the whole job in the
// header, not one pass at a time: the second pass carries on from where the
// first left off instead of resetting to its own small total.
func TestProgressUpdater_CountsAcrossPasses(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	task := progressingTask(t, StatePreparing, 0, 0, 0)
	require.NoError(t, dm.AddTask(task))

	// The second of two passes: the first pass's 4000 bytes are behind it,
	// and the whole job is 10000.
	updater := &progressUpdater{
		task: task, dm: dm,
		fileBytes:    make(map[string]int64),
		fileProgress: make(map[string]struct{ current, total int64 }),
		progressBase: 0.4, progressSpan: 0.6,
		overallBase: 4000, overallTotal: 10000,
	}

	_, err := updater.Write([]byte(`{"type":"start","overall_total":6000}` + "\n" +
		`{"type":"file_progress","file":"b","current":1500,"total":6000}` + "\n"))
	require.NoError(t, err)

	downloaded, total, _ := task.ProgressBytes()
	require.Equal(t, int64(10000), total, "the header shows the whole job, not the pass")
	require.Equal(t, int64(5500), downloaded, "the prior pass's bytes are carried into the count")
}
