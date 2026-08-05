package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
)

// screenshotThumbSize is the size GOG serves the thumbnail rendition at, so it
// is shown as it comes rather than scaled.
var screenshotThumbSize = fyne.NewSize(112, 63)

// screenshotLargeSize is the size of the rendition shown when a thumbnail is
// tapped.
var screenshotLargeSize = fyne.NewSize(748, 421)

// screenshotThumb is one picture in the strip. It is a widget of its own so it
// can be tapped, and so tests can find it.
type screenshotThumb struct {
	widget.BaseWidget
	picture *canvas.Image
	onTap   func()
}

func newScreenshotThumb(onTap func()) *screenshotThumb {
	picture := canvas.NewImageFromResource(nil)
	picture.FillMode = canvas.ImageFillContain
	picture.SetMinSize(screenshotThumbSize)

	thumb := &screenshotThumb{picture: picture, onTap: onTap}
	thumb.ExtendBaseWidget(thumb)
	return thumb
}

func (t *screenshotThumb) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.picture)
}

func (t *screenshotThumb) Tapped(_ *fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *screenshotThumb) Cursor() desktop.Cursor { return desktop.PointerCursor }

// screenshotStrip is a row of the pictures from a game's store page. It returns
// nil when there is nothing to show, so the caller can leave the space to the
// rest of the pane. stillWanted is asked before a picture is shown, because the
// user may have selected another game while it was in flight.
func screenshotStrip(shots []client.Screenshot, covers *coverCache,
	stillWanted func() bool, open func(client.Screenshot),
) fyne.CanvasObject {
	if len(shots) == 0 {
		return nil
	}

	thumbs := make([]fyne.CanvasObject, 0, len(shots))
	for _, shot := range shots {
		shot := shot
		thumb := newScreenshotThumb(func() { open(shot) })
		thumbs = append(thumbs, thumb)

		covers.loadURL(shot.ThumbnailURL, stillWanted, func(data []byte) {
			thumb.picture.Resource = fyne.NewStaticResource(shot.ThumbnailURL, data)
			thumb.picture.Refresh()
		})
	}

	// More pictures than fit are reached by scrolling rather than by shrinking
	// them, and the strip is only as tall as one row.
	strip := container.NewHScroll(container.NewHBox(thumbs...))
	strip.SetMinSize(fyne.NewSize(0, screenshotThumbSize.Height+theme.Padding()*2))
	return strip
}

// showScreenshot opens one picture at the larger rendition.
func showScreenshot(win fyne.Window, shot client.Screenshot, covers *coverCache) {
	picture := canvas.NewImageFromResource(nil)
	picture.FillMode = canvas.ImageFillContain
	picture.SetMinSize(screenshotLargeSize)

	popup := dialog.NewCustom("Screenshot", "Close", container.NewPadded(picture), win)
	shown := true
	popup.SetOnClosed(func() { shown = false })

	covers.loadURL(shot.LargeURL, func() bool { return shown }, func(data []byte) {
		picture.Resource = fyne.NewStaticResource(shot.LargeURL, data)
		picture.Refresh()
	})
	popup.Show()
}
