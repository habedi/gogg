package gui

import (
	"context"
	"encoding/json"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// putGenres stores a lookup whose genres the library should pick up.
func putGenres(t *testing.T, gameID int, genres ...string) {
	t.Helper()
	data, err := json.Marshal(client.GameMetadata{Genres: genres})
	require.NoError(t, err)
	require.NoError(t, db.PutGameMetadata(context.Background(), gameID, metadataFormat, data))
}

// What GOG's store said a game is becomes a way to filter the library.
func TestLibrary_FiltersByGenreFromStoredLookups(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 3)
	putGenres(t, 1, "Role-playing", "Adventure")
	putGenres(t, 2, "Strategy")
	loadGameGenres()

	lt.searchEntry.SetText("genre:role")
	require.Len(t, lt.listed(), 1)
	require.Equal(t, 1, lt.listed()[0].ID)

	lt.searchEntry.SetText("genre:strategy")
	require.Len(t, lt.listed(), 1)
	require.Equal(t, 2, lt.listed()[0].ID)

	// A game whose store page was never looked up simply has no genre.
	lt.searchEntry.SetText("genre:puzzle")
	require.Empty(t, lt.listed())
}

// The filter dialog offers the genre beside the tag, the same filter as the
// typed one.
func TestFiltersDialog_OffersGenre(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _, win := newLibraryFixtureInWindow(t, 2)
		win.SetContent(lt.content)

		test.Tap(buttonWithLabel(lt.content, "Filters"))
		overlay := topOverlay(t)
		require.NotNil(t, overlay)

		labels := formLabels(overlay)
		require.Contains(t, labels, "Genre")
	})
}
