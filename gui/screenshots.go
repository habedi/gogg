package gui

import (
	"fmt"
	"path"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// suggestedPictureName names a saved picture after the address it came from,
// so a folder of saved screenshots does not become a folder of "download (7)".
func suggestedPictureName(url string) string {
	name := ""
	if parsed := parseURL(url); parsed != nil {
		name = path.Base(parsed.Path)
	}
	if name == "" || name == "." || name == "/" {
		return "picture.jpg"
	}
	return name
}

// screenshotLargeSize is the size GOG serves the larger rendition at.
var screenshotLargeSize = fyne.NewSize(748, 421)

// pictureViewer is the dialog's body. It is a widget so it can take focus and
// turn the arrow keys into moves, the way the gallery under it does.
type pictureViewer struct {
	widget.BaseWidget

	content fyne.CanvasObject
	onMove  func(step int)
}

func (v *pictureViewer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(v.content)
}

func (v *pictureViewer) TypedKey(event *fyne.KeyEvent) {
	switch event.Name {
	case fyne.KeyLeft:
		v.onMove(-1)
	case fyne.KeyRight:
		v.onMove(1)
	}
}

func (v *pictureViewer) TypedRune(_ rune) {}
func (v *pictureViewer) FocusGained()     {}
func (v *pictureViewer) FocusLost()       {}
func (v *pictureViewer) AcceptsTab() bool { return false }

// showPictures opens the pictures on their own, larger than the gallery shows
// them, starting at the given one. The arrows either side and the arrow keys
// move through them without closing the dialog; the ends hold rather than wrap.
func showPictures(win fyne.Window, pictures []galleryPicture, start int, covers *coverCache) {
	if start < 0 || start >= len(pictures) {
		return
	}

	large := newGalleryImage(screenshotLargeSize, nil)
	counter := widget.NewLabel("")
	counter.Alignment = fyne.TextAlignCenter

	open := true
	current := -1
	prevBtn := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), nil)
	nextBtn := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), nil)

	// What the picture on show was fetched as, kept so saving it writes the
	// bytes GOG served rather than fetching them again.
	var currentData []byte
	saveBtn := widget.NewButtonWithIcon("Save...", theme.DocumentSaveIcon(), nil)

	show := func(index int) {
		if index < 0 || index >= len(pictures) || index == current {
			return
		}
		current = index
		counter.SetText(fmt.Sprintf("%d / %d", index+1, len(pictures)))
		if index == 0 {
			prevBtn.Disable()
		} else {
			prevBtn.Enable()
		}
		if index == len(pictures)-1 {
			nextBtn.Disable()
		} else {
			nextBtn.Enable()
		}

		// Cleared first, so a slow fetch shows as an empty frame rather than
		// as the previous picture under the next one's number. There is
		// nothing to save until the new one lands.
		large.clear()
		currentData = nil
		saveBtn.Disable()
		picture := pictures[index]
		covers.loadURL(picture.LargeURL, func() bool { return open && current == index },
			func(data []byte) {
				large.setPicture(data, picture.Faded)
				currentData = data
				saveBtn.Enable()
			})

		// The neighbours are warmed up while this one is looked at, so moving
		// on does not mean watching another fetch.
		for _, neighbour := range []int{index - 1, index + 1} {
			if neighbour >= 0 && neighbour < len(pictures) {
				covers.loadURL(pictures[neighbour].LargeURL,
					func() bool { return open }, func([]byte) {})
			}
		}
	}
	move := func(step int) { show(current + step) }
	prevBtn.OnTapped = func() { move(-1) }
	nextBtn.OnTapped = func() { move(1) }

	saveBtn.OnTapped = func() {
		data := currentData
		if len(data) == 0 {
			return
		}
		fd := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			defer func() { _ = writer.Close() }()
			if _, err := writer.Write(data); err != nil {
				showErrorDialog(win, "Could not save the picture", err)
			}
		}, win)
		fd.SetFileName(suggestedPictureName(pictures[current].LargeURL))
		fd.Resize(fileDialogSize)
		fd.Show()
	}

	bottom := container.NewBorder(nil, nil, saveBtn, nil, counter)
	body := container.NewBorder(nil, bottom,
		container.NewCenter(prevBtn), container.NewCenter(nextBtn),
		container.NewPadded(large))

	// A single picture has nowhere to go, so the ways to go are not offered.
	if len(pictures) < 2 {
		prevBtn.Hide()
		nextBtn.Hide()
		counter.Hide()
	}

	viewer := &pictureViewer{content: body, onMove: move}
	viewer.ExtendBaseWidget(viewer)

	popup := dialog.NewCustom("Picture", "Close", viewer, win)
	popup.SetOnClosed(func() { open = false })

	// Shown before the picture is asked for, so a fetch that answers quickly
	// cannot land in a dialog that is still opening.
	popup.Show()
	win.Canvas().Focus(viewer)
	show(start)
}
