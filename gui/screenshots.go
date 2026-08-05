package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
)

// screenshotLargeSize is the size GOG serves the larger rendition at.
var screenshotLargeSize = fyne.NewSize(748, 421)

// showPicture opens one picture on its own, larger than the gallery shows it.
func showPicture(win fyne.Window, picture galleryPicture, covers *coverCache) {
	large := newGalleryImage(screenshotLargeSize, nil)
	popup := dialog.NewCustom("Picture", "Close", container.NewPadded(large), win)
	open := true
	popup.SetOnClosed(func() { open = false })

	// Shown before the picture is asked for, so a fetch that answers quickly
	// cannot land in a dialog that is still opening.
	popup.Show()
	covers.loadURL(picture.LargeURL, func() bool { return open }, func(data []byte) {
		large.setPicture(data, picture.Faded)
	})
}
