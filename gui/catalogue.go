package gui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type CopyableLabel struct {
	widget.Label
}

func NewCopyableLabel() *CopyableLabel {
	cl := &CopyableLabel{}
	cl.ExtendBaseWidget(cl)
	return cl
}

func (cl *CopyableLabel) Tapped(_ *fyne.PointEvent) {
	fyne.CurrentApp().Clipboard().SetContent(cl.Text)
	if _, err := strconv.Atoi(cl.Text); err == nil {
		if len(cl.Text) > 4 {
			fyne.CurrentApp().SendNotification(fyne.NewNotification("Copied",
				fmt.Sprintf("Game ID %s copied to clipboard", cl.Text)))
		}
	}
}
