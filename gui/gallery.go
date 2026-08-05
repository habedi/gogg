package gui

import (
	"bytes"
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// maxViewerHeight caps how much of the pane the artwork takes, so a wide window
// does not leave the facts below the fold.
const maxViewerHeight = 260

// galleryThumbSize is the size GOG serves the screenshot thumbnails at, so they
// are shown as they come rather than scaled.
var galleryThumbSize = fyne.NewSize(112, 63)

// galleryPicture is one picture in the gallery.
type galleryPicture struct {
	ThumbnailURL string
	LargeURL     string
	// Faded marks GOG's background artwork, which has a fade baked into every
	// rendition of it and has to be cropped before it is shown.
	Faded bool
}

// galleryPicturesFor gathers everything there is to see of a game: its own
// artwork and the pictures from its store page. The artwork goes in the middle,
// and its position is returned because that is what the gallery opens on.
func galleryPicturesFor(game db.Game, shots []client.Screenshot) ([]galleryPicture, int) {
	pictures := make([]galleryPicture, 0, len(shots)+1)

	cover := galleryPicture{}
	banner := coverSourceFor(game, coverBanner)
	if banner.URL != "" {
		cover = galleryPicture{
			ThumbnailURL: coverSourceFor(game, coverThumbnail).URL,
			LargeURL:     banner.URL,
			Faded:        banner.Faded,
		}
	}

	middle := len(shots) / 2
	for i, shot := range shots {
		if i == middle && cover.LargeURL != "" {
			pictures = append(pictures, cover)
		}
		pictures = append(pictures, galleryPicture{
			ThumbnailURL: shot.ThumbnailURL,
			LargeURL:     shot.LargeURL,
		})
	}
	if cover.LargeURL != "" && middle == len(shots) {
		pictures = append(pictures, cover)
	}

	selected := middle
	if cover.LargeURL == "" {
		selected = 0
	}
	return pictures, selected
}

// gameGallery shows one picture at a time with the rest as thumbnails below it.
// The arrow keys and the thumbnails both move through them.
type gameGallery struct {
	widget.BaseWidget

	covers *coverCache
	win    fyne.Window

	viewer  *galleryImage
	strip   *fyne.Container
	scroll  *container.Scroll
	content *fyne.Container

	pictures []galleryPicture
	thumbs   []*galleryImage
	selected int
	// shown counts the games the gallery has been given, so a picture fetched
	// for a game that has since been replaced can be turned away.
	shown int
}

func newGameGallery(covers *coverCache, win fyne.Window) *gameGallery {
	gallery := &gameGallery{covers: covers, win: win}

	gallery.viewer = newGalleryImage(artworkSize, func() {
		gallery.takeFocus()
		gallery.openSelected()
	})
	gallery.strip = container.NewHBox()
	gallery.scroll = container.NewHScroll(gallery.strip)
	gallery.scroll.SetMinSize(fyne.NewSize(0, galleryThumbSize.Height+theme.Padding()*3))
	gallery.content = container.NewVBox(container.NewPadded(gallery.viewer), gallery.scroll)

	gallery.ExtendBaseWidget(gallery)
	gallery.show(nil, 0)
	return gallery
}

func (g *gameGallery) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.content)
}

// Resize gives the artwork the width the pane has to spare, keeping it 16:9. A
// fixed size either leaves the pane half empty or forces it wider than it needs
// to be.
func (g *gameGallery) Resize(size fyne.Size) {
	width := size.Width - theme.Padding()*4
	if width > artworkSize.Width {
		height := width * 9 / 16
		if height > maxViewerHeight {
			height = maxViewerHeight
			width = height * 16 / 9
		}
		// Only when it makes a difference: setting a minimum size asks for
		// another layout, and matching sizes would go round for ever.
		if current := g.viewer.picture.MinSize(); absDiff(current.Height, height) > 1 {
			g.viewer.picture.SetMinSize(fyne.NewSize(width, height))
		}
	}
	g.BaseWidget.Resize(size)
}

func absDiff(a, b float32) float32 {
	if a > b {
		return a - b
	}
	return b - a
}

// show replaces what the gallery is holding, opening on the given picture.
func (g *gameGallery) show(pictures []galleryPicture, selected int) {
	g.shown++
	g.pictures = pictures
	g.selected = -1

	g.thumbs = nil
	thumbs := make([]fyne.CanvasObject, 0, len(pictures))
	for i := range pictures {
		index := i
		thumb := newGalleryImage(galleryThumbSize, func() {
			g.takeFocus()
			g.selectPicture(index)
		})
		g.thumbs = append(g.thumbs, thumb)
		thumbs = append(thumbs, thumb)
	}
	g.strip.Objects = thumbs
	g.strip.Refresh()

	// A single picture has nothing to choose between, and none at all leaves the
	// space to the rest of the pane.
	if len(pictures) > 1 {
		g.scroll.Show()
	} else {
		g.scroll.Hide()
	}
	if len(pictures) == 0 {
		g.viewer.clear()
		g.viewer.Hide()
		g.Refresh()
		return
	}

	g.viewer.Show()
	g.selectPicture(selected)
	g.Refresh()

	// The pictures are asked for once the gallery is on screen, so a fetch that
	// answers quickly cannot land in a widget that is still being built.
	for i, picture := range pictures {
		g.loadInto(g.thumbs[i], picture.ThumbnailURL, picture.Faded)
	}
}

// selectPicture puts one of the pictures in the viewer and marks its thumbnail.
func (g *gameGallery) selectPicture(index int) {
	if index < 0 || index >= len(g.pictures) || index == g.selected {
		return
	}
	g.selected = index

	for i, thumb := range g.thumbs {
		thumb.setSelected(i == index)
	}
	g.scrollTo(index)
	g.loadInto(g.viewer, g.pictures[index].LargeURL, g.pictures[index].Faded)
}

// loadInto fetches a picture and puts it in an image, unless the gallery has
// moved on by the time it arrives.
func (g *gameGallery) loadInto(target *galleryImage, url string, faded bool) {
	if g.covers == nil || url == "" {
		return
	}
	wanted := g.shown
	g.covers.loadURL(url, func() bool { return g.shown == wanted }, func(data []byte) {
		target.setPicture(data, faded)
	})
}

// scrollTo keeps the selected thumbnail in view as the selection moves along the
// strip.
func (g *gameGallery) scrollTo(index int) {
	step := galleryThumbSize.Width + theme.Padding()
	middle := float32(index)*step - g.scroll.Size().Width/2 + step/2
	if middle < 0 {
		middle = 0
	}
	g.scroll.Offset.X = middle
	g.scroll.Refresh()
}

// openSelected shows the picture in the viewer at the size GOG serves it.
func (g *gameGallery) openSelected() {
	if g.win == nil || g.selected < 0 || g.selected >= len(g.pictures) {
		return
	}
	showPicture(g.win, g.pictures[g.selected], g.covers)
}

func (g *gameGallery) takeFocus() {
	if canvas := fyne.CurrentApp().Driver().CanvasForObject(g); canvas != nil {
		canvas.Focus(g)
	}
}

// TypedKey moves through the pictures with the arrow keys. The ends hold rather
// than wrap around, so holding a key down cannot cycle forever.
func (g *gameGallery) TypedKey(event *fyne.KeyEvent) {
	switch event.Name {
	case fyne.KeyLeft:
		g.selectPicture(g.selected - 1)
	case fyne.KeyRight:
		g.selectPicture(g.selected + 1)
	}
}

func (g *gameGallery) TypedRune(_ rune) {}

// The arrow keys move through the pictures, so the gallery outlines what they
// will move, the same way a thumbnail shows it is the one on view.
func (g *gameGallery) FocusGained()     { g.viewer.setSelected(true) }
func (g *gameGallery) FocusLost()       { g.viewer.setSelected(false) }
func (g *gameGallery) AcceptsTab() bool { return false }
func (g *gameGallery) Tapped(_ *fyne.PointEvent) {
	g.takeFocus()
}

// galleryImage is one tappable picture, in the viewer or in the strip.
type galleryImage struct {
	widget.BaseWidget

	picture *canvas.Image
	border  *canvas.Rectangle
	onTap   func()
}

func newGalleryImage(size fyne.Size, onTap func()) *galleryImage {
	picture := canvas.NewImageFromResource(nil)
	picture.FillMode = canvas.ImageFillContain
	picture.SetMinSize(size)

	border := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	border.StrokeColor = theme.Color(theme.ColorNamePrimary)
	border.StrokeWidth = 0

	tappable := &galleryImage{picture: picture, border: border, onTap: onTap}
	tappable.ExtendBaseWidget(tappable)
	return tappable
}

func (i *galleryImage) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(i.border, i.picture))
}

func (i *galleryImage) Tapped(_ *fyne.PointEvent) {
	if i.onTap != nil {
		i.onTap()
	}
}

func (i *galleryImage) Cursor() desktop.Cursor { return desktop.PointerCursor }

// setSelected outlines the picture the gallery is showing.
func (i *galleryImage) setSelected(selected bool) {
	if selected {
		i.border.StrokeWidth = 2
	} else {
		i.border.StrokeWidth = 0
	}
	i.border.Refresh()
}

// setPicture shows what was fetched, cropping GOG's fade out of its artwork.
func (i *galleryImage) setPicture(data []byte, faded bool) {
	if !faded {
		i.picture.Image = nil
		i.picture.Resource = fyne.NewStaticResource("picture", data)
		i.picture.Refresh()
		return
	}

	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	i.picture.Resource = nil
	i.picture.Image = cropArtwork(decoded)
	i.picture.Refresh()
}

func (i *galleryImage) clear() {
	i.picture.Image = nil
	i.picture.Resource = nil
	i.picture.Refresh()
}
