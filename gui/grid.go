package gui

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
)

// prefGridView remembers whether the library was left showing covers.
const prefGridView = "library.gridView"

// coverSize is the space a cover takes in the grid. GOG artwork is wide, so the
// cells are too.
// The tiles GOG serves are 16:9, so the cells are too.
var coverSize = fyne.NewSize(280, 158)

const (
	// artworkKeepFraction is how much of GOG's background art still has a
	// picture in it. GOG fades the rest to white so the image blends into its
	// own pages, and that fade is baked into every rendition of the asset.
	artworkKeepFraction = 0.5
	// artworkAspect is the shape a cell wants what is left to be.
	artworkAspect = 16.0 / 9.0
)

// cropArtwork drops the faded strip and centres what remains.
func cropArtwork(img image.Image) image.Image {
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return img
	}

	bounds := img.Bounds()
	height := int(float64(bounds.Dy()) * artworkKeepFraction)
	width := int(float64(height) * artworkAspect)
	if height < 1 || width < 1 || width > bounds.Dx() {
		return img
	}

	left := bounds.Min.X + (bounds.Dx()-width)/2
	return sub.SubImage(image.Rect(left, bounds.Min.Y, left+width, bounds.Min.Y+height))
}

// gameCell is one game in the cover grid. It knows which game it is showing so
// a cover that arrives late can be turned away: cells are recycled while the
// artwork for the game they used to show is still being fetched.
type gameCell struct {
	widget.BaseWidget

	gameID    int
	check     *widget.Check
	title     *widget.Label
	platforms *widget.Label
	cover     *canvas.Image
	badges    *statusBadges
}

func newGameCell() fyne.CanvasObject {
	cell := &gameCell{
		check:     widget.NewCheck("", nil),
		title:     widget.NewLabel("Game Title"),
		platforms: widget.NewLabel("Windows"),
		cover:     canvas.NewImageFromResource(theme.FileImageIcon()),
		badges:    newStatusBadges(),
	}
	cell.title.Truncation = fyne.TextTruncateEllipsis
	cell.title.TextStyle = fyne.TextStyle{Bold: true}
	// The platform line stays quieter than the title by weight alone: the
	// title is bold, this is not. Low importance would dim it into Fyne's
	// disabled gray, which is too faint to read on either background.
	cell.platforms.Truncation = fyne.TextTruncateEllipsis
	cell.cover.FillMode = canvas.ImageFillContain
	// GridWrap sizes every cell from this template, so the artwork asks for the
	// room it needs rather than collapsing to an icon.
	cell.cover.SetMinSize(coverSize)

	cell.ExtendBaseWidget(cell)
	return cell
}

func (c *gameCell) CreateRenderer() fyne.WidgetRenderer {
	titleRow := container.NewBorder(nil, nil, c.check,
		container.NewHBox(c.badges.downloaded, c.badges.update), c.title)
	// A download in flight draws its bar across the cell, where the eye already
	// is; the platforms sit under the title, quieter than it.
	caption := container.NewVBox(c.badges.progress, titleRow, c.platforms)
	return widget.NewSimpleRenderer(container.NewBorder(nil, caption, nil, nil, c.cover))
}

// platformCaption names the platforms a game offers, for the line under its
// title in the grid.
func platformCaption(s *libraryState, game db.Game) string {
	facts := s.factsOf(game)
	names := make([]string, 0, len(facts.platforms))
	for _, platform := range facts.platforms {
		names = append(names, platformInWords(platform))
	}
	return strings.Join(names, " · ")
}

// showing reports whether this cell is still displaying the given game.
func (c *gameCell) showing(gameID int) bool { return c.gameID == gameID }

// bindGameCell points a recycled cell at a game.
func bindGameCell(cell *gameCell, game db.Game, rb rowBinding) {
	if rb.state == nil {
		rb.state = newLibraryState()
	}
	sel, covers, dm, onToggle := rb.sel, rb.covers, rb.dm, rb.onToggle
	// Anything that refreshes the grid rebinds every visible cell. Only a cell
	// that has been pointed at a different game needs its artwork replaced;
	// throwing it away on every refresh makes the whole grid blink.
	sameGame := cell.gameID == game.ID
	cell.gameID = game.ID
	cell.title.SetText(game.Title)
	cell.badges.show(game.ID, dm, rb.state, rb.win)

	// A game with downloads names its platforms; one with none says so in
	// italics, rather than leaving a blank that reads as missing data. GOG
	// serves no installer files for online, Galaxy-delivered titles, so gogg
	// has nothing to download or a platform to name for them.
	if caption := platformCaption(rb.state, game); caption != "" {
		cell.platforms.SetText(caption)
		cell.platforms.TextStyle = fyne.TextStyle{}
	} else {
		cell.platforms.SetText("No downloads")
		cell.platforms.TextStyle = fyne.TextStyle{Italic: true}
	}
	cell.platforms.Refresh()
	cell.platforms.Show()

	bindCheck(cell.check, sel.has(game.ID), func(checked bool) {
		sel.set(game.ID, checked)
		if onToggle != nil {
			onToggle()
		}
	})
	// A game with nothing to download cannot be picked for one.
	if rb.state != nil && !rb.state.downloadable(game) {
		cell.check.Disable()
	} else {
		cell.check.Enable()
	}

	if sameGame && cell.cover.Image != nil {
		return // already showing this game's artwork
	}

	// Start from the placeholder so a recycled cell never shows the previous
	// game's artwork while the new one is on its way.
	cell.cover.Resource = theme.FileImageIcon()
	cell.cover.Image = nil
	cell.cover.Refresh()

	if covers == nil {
		return
	}
	covers.load(game, coverBanner, cell.showing, func(data []byte, source coverSource) {
		decoded, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return
		}
		if source.Faded {
			decoded = cropArtwork(decoded)
		}
		cell.cover.Resource = nil
		cell.cover.Image = decoded
		cell.cover.Refresh()
	})
}
