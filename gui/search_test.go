package gui

import (
	"context"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
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
		lt.state.statuses = map[int]updateStatus{
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
		lt.state.statuses = map[int]updateStatus{1: {Downloaded: true}, 2: {Downloaded: true}}

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
		lt.state.loadTags(db.NewTagRepository(db.GetDB()))
		lt.relist()

		lt.searchEntry.SetText("favorite:yes")
		require.Equal(t, []string{"Game 2"}, listedTitles(t, lt))

		lt.searchEntry.SetText("hidden:no")
		require.Len(t, listedTitles(t, lt), 3, "nothing is hidden yet")

		require.NoError(t, db.AddTag(context.Background(), 3, db.TagHidden))
		lt.state.loadTags(db.NewTagRepository(db.GetDB()))
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
		lt.state.tags = map[int][]string{2: {db.TagHidden}}
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

// The filter dialog shows platforms and languages by their names, and the
// terms it writes use the keys and codes the search matches on.
func TestFilterMapping_NamesInValuesOut(t *testing.T) {
	require.Equal(t, "mac", platformKeyOrAny("macOS"))
	require.Equal(t, "linux", platformKeyOrAny("Linux"))
	require.Equal(t, anyChoice, platformKeyOrAny(anyChoice), "Any is not a filter")

	require.Equal(t, "de", languageCodeOrAny("Deutsch"))
	require.Equal(t, "English", languageNameForCode("en"))
	require.Equal(t, anyChoice, languageCodeOrAny(anyChoice))
	require.Empty(t, languageNameForCode(""), "no stored code shows as Any")
}

// Opening the dialog on an existing query shows the names, and applying it
// writes the query back with the keys and codes intact: a round trip.
func TestFiltersDialog_RoundTripsNamesAndValues(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	entry := widget.NewEntry()
	entry.SetText("witcher platform:mac lang:de")
	btn := newFiltersButton(entry, func() {})
	win.SetContent(btn)

	test.Tap(btn)
	// The dialog parents to the driver's first window, which is the one the
	// test app made, not necessarily our handle to it.
	parent := fyne.CurrentApp().Driver().AllWindows()[0]
	overlay := parent.Canvas().Overlays().Top()
	require.NotNil(t, overlay, "the dialog opens")

	var platform, language *widget.Select
	for _, sel := range widgetsOfType[*widget.Select](overlay) {
		switch {
		case slices.Contains(sel.Options, "Windows"):
			platform = sel
		case slices.Contains(sel.Options, "English"):
			language = sel
		}
	}
	require.NotNil(t, platform, "the platform select shows names")
	require.NotNil(t, language, "the language select shows names")
	require.Equal(t, "macOS", platform.Selected, "the stored key shows as its name")
	require.Equal(t, "Deutsch", language.Selected)

	apply := buttonWithLabel(overlay, "Apply")
	require.NotNil(t, apply)
	apply.OnTapped()

	require.Contains(t, entry.Text, "platform:mac", "the applied term keeps the key")
	require.Contains(t, entry.Text, "lang:de", "and the code")
	require.Contains(t, entry.Text, "witcher", "and the words the user typed")
}
