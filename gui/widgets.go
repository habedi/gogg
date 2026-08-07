package gui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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

// fileDialogSize is the room every file and folder dialog opens with. Fyne's
// default shows barely a few rows of files; this gives the listing real room,
// and Fyne clamps it to the window when the window is smaller.
var fileDialogSize = fyne.NewSize(920, 700)

// emptyState is what a pane, a list, or a tab shows when there is nothing in
// it: an icon, a heading, a line about what would fill it, and where there is
// something to do about it, a button. One shape for all of them, so an empty
// library and an empty download list do not read as different kinds of nothing.
func emptyState(icon fyne.Resource, heading, detail string, action fyne.CanvasObject) fyne.CanvasObject {
	parts := []fyne.CanvasObject{
		container.NewCenter(widget.NewIcon(icon)),
		widget.NewLabelWithStyle(heading, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
	}
	if detail != "" {
		parts = append(parts, widget.NewLabelWithStyle(detail, fyne.TextAlignCenter, fyne.TextStyle{}))
	}
	if action != nil {
		parts = append(parts, container.NewCenter(action))
	}
	return container.NewCenter(container.NewVBox(parts...))
}

// iconButton is a button whose whole meaning is its icon. Fyne has no tooltips,
// so it says what it does while the pointer rests on it: an icon nobody can
// name is a button nobody can use.
type iconButton struct {
	widget.Button

	tip   string
	shown *widget.PopUp
}

func newIconButton(icon fyne.Resource, tip string, tapped func()) *iconButton {
	button := &iconButton{tip: tip}
	button.Icon = icon
	button.OnTapped = tapped
	button.Importance = widget.LowImportance
	button.ExtendBaseWidget(button)
	return button
}

// MouseIn shows what the button is for. The button's own hover highlight still
// happens: this is on top of it, not instead of it.
func (b *iconButton) MouseIn(event *desktop.MouseEvent) {
	b.Button.MouseIn(event)

	if b.tip == "" || b.shown != nil {
		return
	}
	driver := fyne.CurrentApp().Driver()
	canvas := driver.CanvasForObject(b)
	if canvas == nil {
		return
	}

	b.shown = widget.NewPopUp(widget.NewLabel(b.tip), canvas)
	b.shown.ShowAtPosition(driver.AbsolutePositionForObject(b).
		Add(fyne.NewPos(0, b.Size().Height)))
}

func (b *iconButton) MouseOut() {
	b.Button.MouseOut()

	if b.shown != nil {
		b.shown.Hide()
		b.shown = nil
	}
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
