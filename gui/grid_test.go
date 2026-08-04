package gui

import (
	"image"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func TestNewGameCell_ShowsTitleAndSelection(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	sel := newGameSelection()
	sel.set(1, true)

	cell := newGameCell().(*gameCell)
	bindGameCell(cell, db.Game{ID: 1, Title: "Selected Game"}, sel, nil, nil)

	require.Equal(t, "Selected Game", cell.title.Text)
	require.True(t, cell.check.Checked)
}

// Cells are recycled while covers are still being fetched, so a cell must not
// carry the previous game's selection or artwork.
func TestBindGameCell_RecyclingDoesNotLeakSelection(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	sel := newGameSelection()
	sel.set(1, true)

	cell := newGameCell().(*gameCell)
	bindGameCell(cell, db.Game{ID: 1, Title: "One"}, sel, nil, nil)
	bindGameCell(cell, db.Game{ID: 2, Title: "Two"}, sel, nil, nil)

	require.True(t, sel.has(1), "recycling must not deselect the game the cell used to show")
	require.False(t, sel.has(2))
	require.False(t, cell.check.Checked)
	require.Equal(t, 2, cell.gameID, "the cell knows which game it is showing now")
}

// A cover that arrives after the cell moved on must be dropped.
func TestGameCell_RefusesACoverForAGameItNoLongerShows(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	cell := newGameCell().(*gameCell)
	sel := newGameSelection()

	bindGameCell(cell, db.Game{ID: 1, Title: "One"}, sel, nil, nil)
	require.True(t, cell.showing(1))

	bindGameCell(cell, db.Game{ID: 2, Title: "Two"}, sel, nil, nil)
	require.False(t, cell.showing(1), "the answer for game 1 is no longer wanted")
	require.True(t, cell.showing(2))
}

func TestLibraryTab_CanSwitchToTheGrid(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 3)

	toggle := buttonWithLabel(lt.content, "Grid View")
	require.NotNil(t, toggle, "the library must offer the cover grid")
	require.Empty(t, widgetsOfType[*widget.GridWrap](lt.content))

	toggle.OnTapped()
	grids := widgetsOfType[*widget.GridWrap](lt.content)
	require.Len(t, grids, 1)
	require.Equal(t, 3, grids[0].Length())
	require.Equal(t, "List View", toggle.Text, "the button offers the way back")

	toggle.OnTapped()
	require.Empty(t, widgetsOfType[*widget.GridWrap](lt.content))
}

// The chosen view is how the library looks next time it opens.
func TestLibraryTab_RemembersTheChosenView(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)
	buttonWithLabel(lt.content, "Grid View").OnTapped()

	require.True(t, app.Preferences().Bool(prefGridView))
}

// Whichever view is showing, the same games are listed.
func TestLibraryTab_GridFollowsTheSearch(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 3)
	buttonWithLabel(lt.content, "Grid View").OnTapped()

	grid := widgetsOfType[*widget.GridWrap](lt.content)[0]
	require.Equal(t, 3, grid.Length())

	lt.searchEntry.SetText("Game 1")
	require.Equal(t, 1, grid.Length())
}

// GridWrap sizes every cell from the template, so a cell that asks for nothing
// produces a wall of thumbnails too small to see.
func TestNewGameCell_IsBigEnoughToShowArtwork(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	min := newGameCell().MinSize()
	require.GreaterOrEqual(t, min.Width, coverSize.Width)
	require.Greater(t, min.Height, coverSize.Height, "the caption sits below the cover")
}

// GOG bakes a fade to white into the bottom of its background art so it blends
// into its own pages: measured across games, contrast collapses from ~25 to ~1
// by the bottom. The cell shows only the part that still has a picture in it.
func TestCropArtwork_KeepsTheTopAndCentresIt(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 639, 361))

	cropped := cropArtwork(source)
	bounds := cropped.Bounds()

	require.Equal(t, 0, bounds.Min.Y, "the crop is anchored at the top, where the art is")
	require.Less(t, bounds.Dy(), source.Bounds().Dy()/2+20, "the faded half is dropped")
	require.InDelta(t, artworkAspect, float64(bounds.Dx())/float64(bounds.Dy()), 0.05)
	require.Equal(t, (639-bounds.Dx())/2, bounds.Min.X, "what is left is centred")
}

func TestCropArtwork_LeavesTinyImagesAlone(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	require.Equal(t, source.Bounds(), cropArtwork(source).Bounds())
}

// Clicking a cover has to do what clicking a row does: show the game.
func TestLibraryTab_SelectingInTheGridShowsTheGame(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 3)
	buttonWithLabel(lt.content, "Grid View").OnTapped()

	grid := widgetsOfType[*widget.GridWrap](lt.content)[0]
	require.NotNil(t, grid.OnSelected, "the grid must report what was clicked")

	grid.OnSelected(0)

	shown, err := lt.selected.Get()
	require.NoError(t, err)
	require.NotNil(t, shown, "the details pane needs a game to show")
	require.Equal(t, "Game 1", shown.(db.Game).Title)

	require.NotNil(t, grid.OnUnselected)
	grid.OnUnselected(0)
	shown, err = lt.selected.Get()
	require.NoError(t, err)
	require.Nil(t, shown)
}

// Anything that refreshes the grid rebinds every visible cell. Throwing the
// artwork away each time makes the whole grid blink.
func TestBindGameCell_KeepsArtworkWhenTheGameHasNotChanged(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	sel := newGameSelection()
	cell := newGameCell().(*gameCell)
	game := db.Game{ID: 1, Title: "One"}

	bindGameCell(cell, game, sel, nil, nil)
	artwork := image.NewRGBA(image.Rect(0, 0, 4, 4))
	cell.cover.Image = artwork
	cell.cover.Resource = nil

	bindGameCell(cell, game, sel, nil, nil)
	require.Equal(t, artwork, cell.cover.Image, "the same game keeps the artwork already on screen")

	bindGameCell(cell, db.Game{ID: 2, Title: "Two"}, sel, nil, nil)
	require.Nil(t, cell.cover.Image, "a different game starts from the placeholder")
	require.NotNil(t, cell.cover.Resource)
}
