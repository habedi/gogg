package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
)

// testCategories is a bar of two categories, the first with two items.
func testCategories(opened *[]string) []xmbCategory {
	open := func(title string) func() fyne.CanvasObject {
		return func() fyne.CanvasObject {
			*opened = append(*opened, title)
			return widget.NewLabel(title)
		}
	}
	return []xmbCategory{
		{
			Title: "Catalogue", Icon: theme.ListIcon(),
			Items: []xmbItem{
				{Title: "Browse library", Detail: func() string { return "3 games" }, Pane: open("Browse library")},
				{Title: "Refresh catalogue", Action: func() { *opened = append(*opened, "refreshed") }},
			},
		},
		{
			Title: "Downloads", Icon: theme.DownloadIcon(),
			Items: []xmbItem{{Title: "Active downloads", Pane: open("Active downloads")}},
		},
	}
}

// The bar opens on the first category with its first item under it.
func TestXMBShell_OpensOnTheFirstCategory(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var opened []string

	shell := newXMBShell(testCategories(&opened))

	require.Equal(t, 0, shell.category)
	require.Equal(t, 0, shell.item)
	require.Len(t, shell.itemKeys, 2, "the items of the selected category are listed")
	require.Equal(t, "3 games", shell.itemKeys[0].detail.Text, "an item says what it holds")
	require.Empty(t, opened, "nothing is opened until it is asked for")
}

// Left and right move along the bar, and the ends hold.
func TestXMBShell_MovesAlongTheBar(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var opened []string

	shell := newXMBShell(testCategories(&opened))

	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	require.Equal(t, 1, shell.category)
	require.Len(t, shell.itemKeys, 1, "the column follows the category")

	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	require.Equal(t, 1, shell.category, "the last category holds")

	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	require.Equal(t, 0, shell.category)
	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	require.Equal(t, 0, shell.category, "the first category holds")
}

// Up and down move through the items of the selected category.
func TestXMBShell_MovesDownTheColumn(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var opened []string

	shell := newXMBShell(testCategories(&opened))

	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	require.Equal(t, 1, shell.item)
	require.Equal(t, "▸", shell.itemKeys[1].marker.Text, "the selected item is marked")
	require.Equal(t, " ", shell.itemKeys[0].marker.Text)

	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	require.Equal(t, 1, shell.item, "the last item holds")

	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	require.Equal(t, 0, shell.item)
	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	require.Equal(t, 0, shell.item, "the first item holds")
}

// Moving to another category starts its column from the top rather than keeping
// a position that means nothing there.
func TestXMBShell_ANewCategoryStartsAtItsFirstItem(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var opened []string

	shell := newXMBShell(testCategories(&opened))
	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	require.Equal(t, 1, shell.item)

	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	require.Equal(t, 0, shell.item)
}

// Enter opens the pane of the selected item, and Escape comes back to the bar.
func TestXMBShell_EnterOpensAndEscapeComesBack(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var opened []string

	shell := newXMBShell(testCategories(&opened))
	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	require.Equal(t, []string{"Browse library"}, opened)
	require.True(t, shell.paneOpen)
	require.False(t, shell.browse.Visible(), "the bar gives the window to the pane")
	require.NotNil(t, buttonWithLabel(shell.content, "Back"), "and there is a way back for the mouse")

	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	require.False(t, shell.paneOpen)
	require.True(t, shell.browse.Visible())
}

// While a pane is open the arrow keys belong to it, not to the bar behind it.
func TestXMBShell_TheBarStaysPutWhileAPaneIsOpen(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var opened []string

	shell := newXMBShell(testCategories(&opened))
	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})

	require.Equal(t, 0, shell.category)
	require.Equal(t, 0, shell.item)
}

// An item with nothing to open does its job on the spot.
func TestXMBShell_AnItemCanActInstead(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var opened []string

	shell := newXMBShell(testCategories(&opened))
	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	require.Equal(t, []string{"refreshed"}, opened)
	require.False(t, shell.paneOpen, "refreshing the catalogue opens nothing")
}

// The mouse works the bar too.
func TestXMBShell_ClickingTheBarAndTheColumn(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var opened []string

	shell := newXMBShell(testCategories(&opened))

	test.Tap(shell.categoryKeys[1])
	require.Equal(t, 1, shell.category)

	test.Tap(shell.itemKeys[0])
	require.Equal(t, []string{"Active downloads"}, opened, "clicking an item opens it")
}

// The column hangs under the category it belongs to.
func TestXMBShell_TheColumnFollowsTheSelectedCategory(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var opened []string

	shell := newXMBShell(testCategories(&opened))
	require.Less(t, shell.indent.MinSize().Width, float32(xmbCategoryWidth),
		"the first category needs no indent")

	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	require.Greater(t, shell.indent.MinSize().Width, float32(xmbCategoryWidth-1),
		"the second column starts past the first icon")
}

// The bar has to fit in the window gogg opens at.
func TestXMBShell_FitsInADefaultWindow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var opened []string

	shell := newXMBShell(appCategories(test.NewWindow(nil),
		&DownloadManager{Tasks: binding.NewUntypedList()}, "test",
		func() fyne.CanvasObject { return widget.NewLabel("library") },
		func() {}))
	require.Empty(t, opened)

	// The defaults in window.go.
	const defaultWidth, defaultHeight = 960, 640
	min := shell.MinSize()
	t.Logf("cross interface minimum size: %.0fx%.0f", min.Width, min.Height)

	require.LessOrEqual(t, min.Width, float32(defaultWidth))
	require.LessOrEqual(t, min.Height, float32(defaultHeight))
}

// The cross interface carries the same sections as the tabs.
func TestAppCategories_CoverTheWholeApp(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	categories := appCategories(test.NewWindow(nil),
		&DownloadManager{Tasks: binding.NewUntypedList()}, "test",
		func() fyne.CanvasObject { return widget.NewLabel("library") },
		func() {})

	var titles []string
	for _, category := range categories {
		titles = append(titles, category.Title)
		require.NotEmpty(t, category.Items, "%s has nothing to do", category.Title)
		require.NotNil(t, category.Icon, "%s has no icon", category.Title)
	}
	require.Equal(t, []string{
		sectionCatalogue, sectionDownloads, sectionFileHashes, sectionSettings, sectionAbout,
	}, titles)
	require.Equal(t, []string{"Catalogue", "Downloads", "File Hashes", "Settings", "About"}, titles,
		"the sections are named the same in the cross interface as in the tabs")

	// Every item is written the same way, rather than some in title case and
	// some not.
	var items []string
	for _, category := range categories {
		for _, item := range category.Items {
			items = append(items, item.Title)
		}
	}
	require.Equal(t, []string{
		"Browse Library", "Refresh Catalogue", "Active Downloads",
		"Hash Files", "Preferences", "About Gogg",
	}, items)
}

// The catalogue pane is asked for when it is opened, so the one rebuilt at login
// is the one that shows.
func TestAppCategories_ReadTheCatalogueWhenItIsOpened(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	shown := widget.NewLabel("first")
	categories := appCategories(test.NewWindow(nil),
		&DownloadManager{Tasks: binding.NewUntypedList()}, "test",
		func() fyne.CanvasObject { return shown },
		func() {})

	shell := newXMBShell(categories)
	shown = widget.NewLabel("rebuilt after login")
	shell.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	var texts []string
	for _, label := range widgetsOfType[*widget.Label](shell.content) {
		texts = append(texts, label.Text)
	}
	require.Contains(t, texts, "rebuilt after login")
}

// Choosing the cross interface in the settings takes effect without a restart.
func TestSettings_SwitchingTheInterface(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	switched := 0
	onInterfaceChanged = func() { switched++ }
	t.Cleanup(func() { onInterfaceChanged = nil })

	ui := SettingsTabUI(test.NewWindow(nil))
	check := checkWithLabel(ui, "Use the cross (XMB) interface instead of tabs")
	require.NotNil(t, check)
	require.False(t, check.Checked, "the tabs stay what gogg opens with")

	check.SetChecked(true)
	require.True(t, fyne.CurrentApp().Preferences().Bool(prefXMB))
	require.Equal(t, 1, switched, "the window is rebuilt there and then")
}

// checkWithLabel finds a check box by what it says.
func checkWithLabel(root fyne.CanvasObject, label string) *widget.Check {
	for _, check := range widgetsOfType[*widget.Check](root) {
		if check.Text == label {
			return check
		}
	}
	return nil
}

// A pane opened over the cross interface has to bring its own background. The
// bar behind it is dark whatever the theme says, so a pane laid straight over
// it puts light-theme text on a dark gradient.
func TestXMBShell_AnOpenPaneCoversTheBarBehindIt(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	shell := newXMBShell([]xmbCategory{{
		Title: "Settings", Items: []xmbItem{{
			Title: "Preferences",
			Pane:  func() fyne.CanvasObject { return widget.NewLabel("a setting") },
		}},
	}})
	shell.activate()

	backdrop := backdropOf(t, shell.paneBox)
	require.Equal(t, theme.Color(theme.ColorNameBackground), backdrop.FillColor,
		"the pane has to sit on the background the rest of the app uses")
}

// backdropOf is the rectangle an open pane is laid on.
func backdropOf(t *testing.T, pane fyne.CanvasObject) *canvas.Rectangle {
	t.Helper()
	rectangles := widgetsOfType[*canvas.Rectangle](pane)
	require.NotEmpty(t, rectangles, "an open pane has nothing behind it")
	return rectangles[0]
}

// The tab already names the section, so the pane under it does not repeat it.
func TestFileTabUI_DoesNotRepeatTheSectionName(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	require.NotContains(t, labelTexts(FileTabUI(win)), sectionFileHashes,
		"the tab says what this is; the pane does not have to say it again")
}
