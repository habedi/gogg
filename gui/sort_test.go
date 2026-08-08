package gui

import (
	"context"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// The library can be put in the order the games were bought, latest first;
// games from catalogues refreshed before gogg recorded the order sink to the
// bottom, and the way back to titles is where the way in was.
func TestLibrary_SortsByPurchaseDate(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		refreshes := captureRefreshes(t)
		lt, _ := newLibraryFixture(t, 3)

		// Game 3 is the latest buy, then Game 2; Game 1 predates the ranks.
		repo := db.NewGameRepository(db.Db)
		blob := `{"title":"G","downloads":[],"extras":[],"dlcs":[]}`
		require.NoError(t, repo.Put(context.Background(),
			db.Game{ID: 3, Title: "Game 3", Data: blob, PurchaseRank: 1}))
		require.NoError(t, repo.Put(context.Background(),
			db.Game{ID: 2, Title: "Game 2", Data: blob, PurchaseRank: 2}))

		test.Tap(iconButtonWithTip(lt.content, tipRefresh))
		refreshes.finish()

		purchase := lt.moreMenu().Items[1]
		require.Equal(t, "Sort by Purchase Date", purchase.Label)
		purchase.Action()

		titles := make([]string, 0, 3)
		for _, game := range lt.listed() {
			titles = append(titles, game.Title)
		}
		require.Equal(t, []string{"Game 3", "Game 2", "Game 1"}, titles,
			"latest buys first, unranked games last")

		back := lt.moreMenu().Items[1]
		require.Equal(t, "Sort by Title", back.Label, "the entry offers the way back")
		back.Action()
		titles = titles[:0]
		for _, game := range lt.listed() {
			titles = append(titles, game.Title)
		}
		require.Equal(t, []string{"Game 1", "Game 2", "Game 3"}, titles)
	})
}
