package gui

import (
	"context"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// listedTitles is what the library is showing.
func listedTitles(t *testing.T, lt *libraryTab) []string {
	t.Helper()
	var titles []string
	for _, game := range lt.listed() {
		titles = append(titles, game.Title)
	}
	return titles
}

// A typed filter narrows the list the same way the dialog does.
func TestLibrarySearch_FiltersOnFields(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)
		// Game 1 is downloaded by the fixture; the others are not.
		updateStatusCache = map[int]updateStatus{
			1: {Downloaded: true},
			2: {Downloaded: false, HasUpdate: true},
			3: {Downloaded: false},
		}

		lt.searchEntry.SetText("downloaded:yes")
		require.Equal(t, []string{"Game 1"}, listedTitles(t, lt))

		lt.searchEntry.SetText("downloaded:no")
		require.ElementsMatch(t, []string{"Game 2", "Game 3"}, listedTitles(t, lt))

		lt.searchEntry.SetText("updates:yes")
		require.Equal(t, []string{"Game 2"}, listedTitles(t, lt))

		lt.searchEntry.SetText("downloaded:no updates:no")
		require.Equal(t, []string{"Game 3"}, listedTitles(t, lt), "terms narrow together")
	})
}

// Words and filters in the same box.
func TestLibrarySearch_WordsAndFieldsTogether(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)
		updateStatusCache = map[int]updateStatus{1: {Downloaded: true}, 2: {Downloaded: true}}

		lt.searchEntry.SetText("game downloaded:yes")
		require.ElementsMatch(t, []string{"Game 1", "Game 2"}, listedTitles(t, lt))

		lt.searchEntry.SetText("Game 2 downloaded:yes")
		require.Equal(t, []string{"Game 2"}, listedTitles(t, lt))
	})
}

// A search box is half-typed most of the time it is looked at.
func TestLibrarySearch_HalfTypedFilterDoesNotEmptyTheLibrary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)
		updateStatusCache = map[int]updateStatus{}

		for _, typed := range []string{"d", "downloaded", "downloaded:", "downloaded:y"} {
			lt.searchEntry.SetText(typed)
			require.NotPanics(t, func() { _ = listedTitles(t, lt) }, "typing %q", typed)
		}

		lt.searchEntry.SetText("downloaded:")
		require.Len(t, listedTitles(t, lt), 3, "an unfinished filter must not hide the library")
	})
}

// What the user marked is asked for the same way as everything else.
func TestLibrarySearch_FindsTaggedGames(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)
		require.NoError(t, db.AddTag(context.Background(), 2, db.TagFavorite))
		loadGameTags()
		lt.relist()

		lt.searchEntry.SetText("favorite:yes")
		require.Equal(t, []string{"Game 2"}, listedTitles(t, lt))

		lt.searchEntry.SetText("hidden:no")
		require.Len(t, listedTitles(t, lt), 3, "nothing is hidden yet")

		require.NoError(t, db.AddTag(context.Background(), 3, db.TagHidden))
		loadGameTags()
		lt.relist()
		require.ElementsMatch(t, []string{"Game 1", "Game 2"}, listedTitles(t, lt))
	})
}

// The dialog and the search box are the same filter, and the dialog offers what
// the box understands rather than half of it.
func TestFilterTerms_ReadAsAQuery(t *testing.T) {
	require.Empty(t, filterTerms(filterChoices{}))
	require.Equal(t, []string{"downloaded:yes"}, filterTerms(filterChoices{Downloaded: true}))
	require.Equal(t, []string{
		"downloaded:yes", "updates:yes", "size:>=10GiB", "size:<=1.5TiB",
		"platform:linux", "lang:de", "tag:finished",
	}, filterTerms(filterChoices{
		Downloaded: true, HasUpdate: true, MinSize: "10 GiB", MaxSize: " 1.5 TiB ",
		Platform: "linux", Language: "de", Tag: " finished ",
	}))
}

// A choice of "any" is not a filter.
func TestFilterTerms_LeavesOutWhatWasNotChosen(t *testing.T) {
	require.Empty(t, filterTerms(filterChoices{Platform: anyChoice, Language: anyChoice}))
}

// Hidden means hidden: the tag put games in a collection of their own but left
// them in every other list as well.
func TestLibraryTab_HiddenGamesStayOutOfTheOtherLists(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)
		gameTags = map[int][]string{2: {db.TagHidden}}
		t.Cleanup(func() { gameTags = map[int][]string{} })
		lt.relist()

		require.ElementsMatch(t, []string{"Game 1", "Game 3"}, listedTitles(t, lt),
			"a hidden game is not in the list")

		lt.sidebar.refresh([]db.Game{{ID: 1, Title: "Game 1"}, {ID: 2, Title: "Game 2"}, {ID: 3, Title: "Game 3"}})
		require.Equal(t, "2", lt.sidebar.buttons["All games"].count.Text,
			"nor counted among all games")
		require.Equal(t, "1", lt.sidebar.buttons["Hidden"].count.Text)

		lt.searchEntry.SetText("hidden:yes")
		require.Equal(t, []string{"Game 2"}, listedTitles(t, lt),
			"and is there when it is what was asked for")
	})
}
