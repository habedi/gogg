package gui

import (
	"testing"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
)

// NewDownloadManager logs and carries on when it cannot build a history file
// path, so both history operations have to cope with not having one.
func TestDownloadManager_NoHistoryPath(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}

	dm.PersistHistory()
	dm.loadHistory()
}
