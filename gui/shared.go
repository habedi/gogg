package gui

import (
	"fyne.io/fyne/v2"
)

// runOnMain schedules fn to run on the main Fyne thread
func runOnMain(fn func()) {
	fyne.Do(fn)
}
