package gui

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
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
