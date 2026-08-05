package gui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// copiedShownFor is how long a copy is confirmed for. Long enough to read, short
// enough not to need dismissing.
const copiedShownFor = 1200 * time.Millisecond

// showCopied confirms a copy beside the thing that was copied. A desktop
// notification for something the user did on purpose, in the window in front of
// them, is an interruption rather than a confirmation, and it competes with the
// notification that says a download has finished.
func showCopied(near fyne.CanvasObject, message string) {
	driver := fyne.CurrentApp().Driver()
	canvas := driver.CanvasForObject(near)
	if canvas == nil {
		return
	}

	popup := widget.NewPopUp(widget.NewLabel(message), canvas)
	popup.ShowAtPosition(driver.AbsolutePositionForObject(near).
		Add(fyne.NewPos(0, near.Size().Height)))
	time.AfterFunc(copiedShownFor, func() { fyne.Do(popup.Hide) })
}

// CopyableLabel is a label that copies its content to the clipboard when tapped.
type CopyableLabel struct {
	widget.Label
}

// NewCopyableLabel creates a new instance of the copyable label with the given text.
func NewCopyableLabel(text string) *CopyableLabel {
	cl := &CopyableLabel{}
	cl.ExtendBaseWidget(cl)
	cl.SetText(text)
	return cl
}

// Tapped is called when a pointer taps this widget.
func (cl *CopyableLabel) Tapped(_ *fyne.PointEvent) {
	fyne.CurrentApp().Clipboard().SetContent(cl.Text)
	showCopied(cl, "Copied")
}

// Cursor is the pointer, because nothing else about a label says it can be
// tapped to copy what it holds.
func (cl *CopyableLabel) Cursor() desktop.Cursor { return desktop.PointerCursor }
