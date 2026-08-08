package gui

import (
	"context"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// The star beside the title marks a game as a favorite, and the Favorites
// collection is those games.
func TestDetailsPane_TheStarMarksAFavorite(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)

	offMain(t, func() {
		require.False(t, lt.pane.favorite.Visible(), "no game, nothing to mark")

		game := lt.listed()[0]
		require.NoError(t, lt.selected.Set(game))
		require.True(t, lt.pane.favorite.Visible())

		test.Tap(lt.pane.favorite)
		tags, err := db.TagsFor(context.Background(), game.ID)
		require.NoError(t, err)
		require.Contains(t, tags, db.TagFavorite)
		require.Equal(t, "Remove from favorites", lt.pane.favorite.tip,
			"the star offers the way back")

		lt.searchEntry.SetText("favorite:yes")
		require.Len(t, lt.listed(), 1)
		require.Equal(t, game.ID, lt.listed()[0].ID)

		lt.searchEntry.SetText("")
		test.Tap(lt.pane.favorite)
		tags, err = db.TagsFor(context.Background(), game.ID)
		require.NoError(t, err)
		require.NotContains(t, tags, db.TagFavorite)
	})
}

// Hiding a game takes it off the list; the Hidden collection is the way back.
func TestDetailsPane_HidingAGameTakesItOffTheList(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)

	offMain(t, func() {
		game := lt.listed()[0]
		require.NoError(t, lt.selected.Set(game))

		test.Tap(lt.pane.hide)
		require.Len(t, lt.listed(), 1, "a hidden game leaves the list")
		require.NotEqual(t, game.ID, lt.listed()[0].ID)

		lt.searchEntry.SetText("hidden:yes")
		require.Len(t, lt.listed(), 1, "the Hidden collection is where it went")
		require.Equal(t, game.ID, lt.listed()[0].ID)

		// The pane still shows the game, so hiding it can be taken back.
		test.Tap(lt.pane.hide)
		lt.searchEntry.SetText("")
		require.Len(t, lt.listed(), 2, "unhiding brings it back")
	})
}

// The Favorites collection only exists once something is in it.
func TestSidebar_FavoritesAppearOnceOneIsMarked(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)

	offMain(t, func() {
		test.Tap(lt.showCollections)
		favorites := lt.sidebar.buttons["Favorites"]
		require.NotNil(t, favorites)
		require.False(t, favorites.Visible(), "an empty collection stays out of the way")

		require.NoError(t, lt.selected.Set(lt.listed()[0]))
		test.Tap(lt.pane.favorite)

		require.True(t, favorites.Visible(), "one favorite is a collection")
	})
}
