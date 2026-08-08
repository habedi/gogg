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
	bindGameCell(cell, db.Game{ID: 1, Title: "Selected Game"}, rowBinding{sel: sel})

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
	bindGameCell(cell, db.Game{ID: 1, Title: "One"}, rowBinding{sel: sel})
	bindGameCell(cell, db.Game{ID: 2, Title: "Two"}, rowBinding{sel: sel})

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

	bindGameCell(cell, db.Game{ID: 1, Title: "One"}, rowBinding{sel: sel})
	require.True(t, cell.showing(1))

	bindGameCell(cell, db.Game{ID: 2, Title: "Two"}, rowBinding{sel: sel})
	require.False(t, cell.showing(1), "the answer for game 1 is no longer wanted")
	require.True(t, cell.showing(2))
}

// Covers are how a person recognises their games, so a fresh library opens on
// the grid; the toggle offers the list and the way back.
func TestLibraryTab_OpensOnTheGridAndCanSwitchToTheList(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 3)

	toggle := iconButtonWithTip(lt.content, tipShowList)
	require.NotNil(t, toggle, "a fresh library opens on the covers")
	grids := widgetsOfType[*activatableGrid](lt.content)
	require.Len(t, grids, 1)
	require.Equal(t, 3, grids[0].Length())

	toggle.OnTapped()
	require.Empty(t, widgetsOfType[*activatableGrid](lt.content))
	require.Equal(t, tipShowCovers, toggle.tip, "the button offers the way back")

	toggle.OnTapped()
	require.Len(t, widgetsOfType[*activatableGrid](lt.content), 1)
}

// The chosen view is how the library looks next time it opens.
func TestLibraryTab_RemembersTheChosenView(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)
	iconButtonWithTip(lt.content, tipShowList).OnTapped()

	require.False(t, app.Preferences().BoolWithFallback(prefGridView, true),
		"choosing the list is remembered")
}

// Whichever view is showing, the same games are listed.
func TestLibraryTab_GridFollowsTheSearch(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 3)

	grid := widgetsOfType[*activatableGrid](lt.content)[0]
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

	grid := widgetsOfType[*activatableGrid](lt.content)[0]
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

	bindGameCell(cell, game, rowBinding{sel: sel})
	artwork := image.NewRGBA(image.Rect(0, 0, 4, 4))
	cell.cover.Image = artwork
	cell.cover.Resource = nil

	bindGameCell(cell, game, rowBinding{sel: sel})
	require.Equal(t, artwork, cell.cover.Image, "the same game keeps the artwork already on screen")

	bindGameCell(cell, db.Game{ID: 2, Title: "Two"}, rowBinding{sel: sel})
	require.Nil(t, cell.cover.Image, "a different game starts from the placeholder")
	require.NotNil(t, cell.cover.Resource)
}

// A game in the grid says as much about itself as one in the list: whether it
// has been downloaded, and whether an update is waiting for it.
func TestBindGameCell_CarriesTheSameBadgesAsARow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	state := newLibraryState()
	state.statuses[1] = updateStatus{Downloaded: true, HasUpdate: true, Diff: []string{"one", "two"}}
	state.statuses[2] = updateStatus{Downloaded: false}

	cell := newGameCell().(*gameCell)
	bindGameCell(cell, db.Game{ID: 1, Title: "One"}, rowBinding{sel: newGameSelection(), state: state})
	require.True(t, cell.badges.downloaded.Visible(), "a downloaded game is marked in the grid too")
	require.True(t, cell.badges.update.Visible())
	require.Equal(t, "2", cell.badges.update.Text)

	bindGameCell(cell, db.Game{ID: 2, Title: "Two"}, rowBinding{sel: newGameSelection(), state: state})
	require.False(t, cell.badges.downloaded.Visible(), "and one that is not, is not")
	require.False(t, cell.badges.update.Visible())
}

// The platform caption must stay readable: hierarchy under the bold title
// comes from weight, never from Fyne's disabled gray, which is too faint on
// either background.
func TestGameCell_PlatformCaptionIsNotDimmed(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	cell := newGameCell().(*gameCell)
	require.Equal(t, widget.MediumImportance, cell.platforms.Importance)
	require.True(t, cell.title.TextStyle.Bold, "the title carries the hierarchy instead")
	require.False(t, cell.platforms.TextStyle.Bold)
}
