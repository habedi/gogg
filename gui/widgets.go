package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

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
	fyne.CurrentApp().SendNotification(
		fyne.NewNotification("Copied to Clipboard", fmt.Sprintf("'%s' was copied.", cl.Text)),
	)
}
