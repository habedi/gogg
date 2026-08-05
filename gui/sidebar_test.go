package gui

import (
	"context"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func collectionTitles(rows []sidebarRow) []string {
	var titles []string
	for _, row := range rows {
		if !row.Header {
			titles = append(titles, row.Title)
		}
	}
	return titles
}

// The collections cover the ways a library is looked at, and each is a query.
func TestLibraryCollections_AreQueries(t *testing.T) {
	rows := libraryCollections()

	require.Equal(t, []string{
		"All games", "Downloaded", "Not downloaded", "Updates", "Favorites", "Hidden",
		"Windows", "macOS", "Linux",
	}, collectionTitles(rows))

	byTitle := map[string]sidebarRow{}
	for _, row := range rows {
		byTitle[row.Title] = row
	}
	require.Empty(t, byTitle["All games"].Query, "everything is the query that asks nothing")
	require.Equal(t, "downloaded:no", byTitle["Not downloaded"].Query)
	require.Equal(t, "platform:linux", byTitle["Linux"].Query)
	require.True(t, byTitle["Hidden"].HideWhenEmpty, "a collection with nothing in it is noise")
	require.False(t, byTitle["All games"].HideWhenEmpty)
}

// Every collection says how much is in it.
func TestCollectionCounts(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	games := []db.Game{
		{ID: 1, Title: "One", Data: richGameData},
		{ID: 2, Title: "Two", Data: richGameData},
		{ID: 3, Title: "Three", Data: `{"title":"T","downloads":[],"extras":[],"dlcs":[]}`},
	}
	updateStatusCache = map[int]updateStatus{
		1: {Downloaded: true},
		2: {Downloaded: true, HasUpdate: true},
	}
	gameTags = map[int][]string{2: {db.TagFavorite}}
	t.Cleanup(func() { gameTags = map[int][]string{} })

	counts := collectionCounts(games, libraryCollections())

	require.Equal(t, 3, counts["All games"])
	require.Equal(t, 2, counts["Downloaded"])
	require.Equal(t, 1, counts["Not downloaded"])
	require.Equal(t, 1, counts["Updates"])
	require.Equal(t, 1, counts["Favorites"])
	require.Equal(t, 0, counts["Hidden"])
	require.Equal(t, 2, counts["Windows"], "the game with no downloads offers no platform")
}

// Picking a collection is the same as typing its query.
func TestSidebar_PickingACollectionWritesItsQuery(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	var picked []string
	sidebar := newLibrarySidebar(libraryCollections(), func(query string) { picked = append(picked, query) })

	test.Tap(sidebar.buttons["Downloaded"])
	test.Tap(sidebar.buttons["Linux"])

	require.Equal(t, []string{"downloaded:yes", "platform:linux"}, picked)
}

// A collection with nothing in it is not offered; the ones always worth having
// stay put.
func TestSidebar_HidesEmptyCollections(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	updateStatusCache = map[int]updateStatus{1: {Downloaded: true}}
	gameTags = map[int][]string{}
	sidebar := newLibrarySidebar(libraryCollections(), func(string) {})

	sidebar.refresh([]db.Game{{ID: 1, Title: "One", Data: richGameData}})

	require.True(t, sidebar.buttons["All games"].Visible())
	require.True(t, sidebar.buttons["Downloaded"].Visible())
	require.False(t, sidebar.buttons["Hidden"].Visible(), "nothing is hidden")
	require.False(t, sidebar.buttons["Updates"].Visible(), "nothing has an update")
	require.Equal(t, "1", sidebar.buttons["All games"].count.Text)
	require.Equal(t, "0", sidebar.buttons["Not downloaded"].count.Text,
		"an empty collection that stays on screen has to say it is empty")
}

// The collections follow the search box, however the query got there.
func TestSidebar_MarksTheCollectionTheSearchIsShowing(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	sidebar := newLibrarySidebar(libraryCollections(), func(string) {})

	sidebar.syncTo("downloaded:yes")
	require.True(t, sidebar.buttons["Downloaded"].title.TextStyle.Bold)
	require.False(t, sidebar.buttons["Not downloaded"].title.TextStyle.Bold)

	sidebar.syncTo("god of war downloaded:yes")
	require.True(t, sidebar.buttons["Downloaded"].title.TextStyle.Bold,
		"words beside the query do not change which collection it is")

	sidebar.syncTo("downloaded:yes updates:yes")
	require.False(t, sidebar.buttons["Downloaded"].title.TextStyle.Bold,
		"a narrowed search is no longer that collection")
}

// The sidebar and the list are the same filter.
func TestLibraryTab_PickingACollectionFiltersTheList(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)
		require.NoError(t, db.AddTag(context.Background(), 2, db.TagFavorite))
		loadGameTags()
		updateStatusCache = map[int]updateStatus{1: {Downloaded: true}}
		test.Tap(lt.showCollections)

		test.Tap(lt.sidebar.buttons["Downloaded"])
		require.Equal(t, "downloaded:yes", lt.searchEntry.Text)
		require.Equal(t, []string{"Game 1"}, listedTitles(t, lt))

		lt.searchEntry.SetText("Game favorite:yes")
		lt.relist()
		require.Equal(t, []string{"Game 2"}, listedTitles(t, lt))
		require.True(t, lt.sidebar.buttons["Favorites"].title.TextStyle.Bold,
			"typing a collection's query marks it too")
	})
}

// The names have to fit: a truncating label reports a tiny minimum size, so
// nothing but a measurement catches "Not downlo…".
func TestSidebar_IsWideEnoughForItsLongestName(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	longest := float32(0)
	for _, row := range libraryCollections() {
		if row.Header {
			continue
		}
		width := fyne.MeasureText(row.Title, theme.TextSize(), fyne.TextStyle{Bold: true}).Width
		if width > longest {
			longest = width
		}
	}

	// The name shares the row with an icon, a count of up to four figures, and
	// the padding around them.
	count := fyne.MeasureText("3120", theme.TextSize(), fyne.TextStyle{}).Width
	need := longest + count + theme.IconInlineSize() + theme.Padding()*8
	require.GreaterOrEqual(t, float32(sidebarWidth), need,
		"the widest collection name must fit without being cut")
}

// The list is what the window is for; the collections are there when asked for.
func TestLibraryTab_CollectionsAreHiddenUntilAskedFor(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)
		require.False(t, lt.sidebar.content.Visible(), "hidden until asked for")

		test.Tap(lt.showCollections)
		require.True(t, lt.sidebar.content.Visible())
		require.True(t, fyne.CurrentApp().Preferences().Bool(prefSidebar), "and remembered")

		test.Tap(lt.showCollections)
		require.False(t, lt.sidebar.content.Visible())
		require.False(t, fyne.CurrentApp().Preferences().Bool(prefSidebar))
	})
}

// Asked for once, they are there the next time gogg opens.
func TestLibraryTab_CollectionsComeBackShown(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		fyne.CurrentApp().Preferences().SetBool(prefSidebar, true)
		t.Cleanup(func() { fyne.CurrentApp().Preferences().SetBool(prefSidebar, false) })

		lt, _ := newLibraryFixture(t, 3)
		require.True(t, lt.sidebar.content.Visible())
		require.Equal(t, "3", lt.sidebar.buttons["All games"].count.Text,
			"and counted, without being toggled first")
	})
}

// Shown, the collections still have to leave the window its default size.
func TestLibraryTab_FitsInADefaultWindowWithCollections(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Rich Game", Data: richGameData}))
		test.Tap(lt.showCollections)

		// The defaults in window.go.
		const defaultWidth, defaultHeight = 960, 640
		min := lt.content.MinSize()
		t.Logf("library with collections: %.0fx%.0f", min.Width, min.Height)

		require.LessOrEqual(t, min.Width, float32(defaultWidth))
		require.LessOrEqual(t, min.Height, float32(defaultHeight))
	})
}

// Showing and hiding the collections has to move the list, not only set a flag
// on the panel: hiding a child leaves its space behind until the pane holding
// it lays itself out again.
func TestLibraryTab_CollectionsFoldAndUnfoldTheLeftPane(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)
		win := test.NewWindow(lt.content)
		t.Cleanup(win.Close)
		win.Resize(fyne.NewSize(1200, 700))

		left := lt.content.(*container.Split).Leading.(*fyne.Container)
		listPane := left.Objects[0]
		folded := listPane.Size().Width
		require.Equal(t, float32(0), listPane.Position().X, "nothing is beside the list yet")

		test.Tap(lt.showCollections)
		require.GreaterOrEqual(t, listPane.Position().X, float32(sidebarWidth),
			"the list has to make room for the collections")
		require.Less(t, listPane.Size().Width, folded)

		test.Tap(lt.showCollections)
		require.Equal(t, float32(0), listPane.Position().X, "and take the room back")
		require.Equal(t, folded, listPane.Size().Width)
	})
}

// A heading with nothing under it describes a group that is not there. When
// every collection in a group is empty they are all hidden, and the heading has
// to go with them.
func TestSidebar_HidesAHeadingWithNothingUnderIt(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	updateStatusCache = map[int]updateStatus{}
	gameTags = map[int][]string{}
	sidebar := newLibrarySidebar(libraryCollections(), func(string) {})

	sidebar.refresh(nil)
	require.False(t, headingLabel(t, sidebar, "Platform").Visible(),
		"no game offers a platform, so the heading has nothing under it")
	require.True(t, headingLabel(t, sidebar, "Library").Visible(),
		"the library collections are always offered")

	sidebar.refresh([]db.Game{{ID: 1, Title: "One", Data: richGameData}})
	require.True(t, headingLabel(t, sidebar, "Platform").Visible(),
		"a game with installers brings the platforms back")
}

// headingLabel is the label that starts a group of collections.
func headingLabel(t *testing.T, sidebar *librarySidebar, title string) *widget.Label {
	t.Helper()
	for _, label := range widgetsOfType[*widget.Label](sidebar.content) {
		if label.Text == strings.ToUpper(title) {
			return label
		}
	}
	t.Fatalf("no %q heading in the collections", title)
	return nil
}
