package gui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// Copying a value says so in the window it happened in. A desktop notification
// for something the user did on purpose, in the window in front of them, is an
// interruption rather than a confirmation.
func TestCopyableLabel_ConfirmsTheCopyInTheWindow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	label := NewCopyableLabel("2.1.0.42")
	win := test.NewWindow(label)
	t.Cleanup(win.Close)
	win.Resize(fyne.NewSize(400, 200))

	test.Tap(label)

	require.Equal(t, "2.1.0.42", app.Clipboard().Content())
	overlay := win.Canvas().Overlays().Top()
	require.NotNil(t, overlay, "the copy has to be confirmed where it happened")
	require.Contains(t, labelTexts(overlay), "Copied")
}

// Nothing about a label says it can be copied, so the pointer has to.
func TestCopyableLabel_ShowsThatItCanBeTapped(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	label := NewCopyableLabel("2.1.0.42")

	cursorable, ok := interface{}(label).(desktop.Cursorable)
	require.True(t, ok, "a label that copies when tapped has to show a pointer")
	require.Equal(t, desktop.PointerCursor, cursorable.Cursor())
}

// The other ways to copy confirm the same way, rather than each having their
// own idea of how to say it.
func TestHashUI_ConfirmsACopyInTheWindow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	win := test.NewWindow(nil)
	t.Cleanup(win.Close)
	ui := HashUI(win)
	win.SetContent(ui)
	win.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))

	test.Tap(buttonWithLabel(ui, "Copy All Results"))

	overlay := win.Canvas().Overlays().Top()
	require.NotNil(t, overlay, "an empty list of hashes still has to answer the button")
	require.Contains(t, labelTexts(overlay), "Nothing to copy")
}

func TestSizeEstimate_ConfirmsACopyInTheWindow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	win := test.NewWindow(nil)
	t.Cleanup(win.Close)
	win.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))

	showSizeEstimate(win, []gameSizeEstimate{{Title: "One", Bytes: 1 << 30}}, 1<<30)
	copyBtn := buttonWithLabel(win.Canvas().Overlays().Top(), "Copy as CSV")
	require.NotNil(t, copyBtn)

	test.Tap(copyBtn)

	require.Contains(t, app.Clipboard().Content(), "One")
	require.Contains(t, labelTexts(win.Canvas().Overlays().Top()), "Copied")
}

// A hardcoded year goes stale the January after it is written.
func TestAboutUI_DoesNotCarryAYearThatGoesStale(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	texts := labelTexts(ShowAboutUI("1.2.3"))

	require.Contains(t, texts, "© Hassan Abedi")
	require.Contains(t, texts, "Version: 1.2.3")
}

// Everywhere that can be empty says so the same way: an icon, a heading, and a
// line about what would fill it. Four places each had their own idea of how
// much to say, from a full explanation down to one unstyled line.
func TestEmptyStates_AllSayItTheSameWay(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	signedOut := LibraryTabUI(win, nil, &DownloadManager{Tasks: binding.NewUntypedList()}, openStores(), func() {})
	// A signed-in library with nothing picked out of the list.
	signedIn, _ := newLibraryFixture(t, 2)

	empty := map[string]fyne.CanvasObject{
		"signed out":       signedOut.content,
		"no downloads yet": DownloadsTabUI(test.NewWindow(nil), &DownloadManager{Tasks: binding.NewUntypedList()}),
		"nothing selected": signedIn.pane.content,
	}

	for what, ui := range empty {
		require.NotEmpty(t, widgetsOfType[*widget.Icon](ui), "%s has no icon", what)
		require.True(t, hasBoldLabel(ui), "%s has no heading", what)
	}
}

func hasBoldLabel(root fyne.CanvasObject) bool {
	for _, label := range widgetsOfType[*widget.Label](root) {
		if label.TextStyle.Bold && label.Text != "" {
			return true
		}
	}
	return false
}

// A button whose whole meaning is its icon has to be able to say what it does.
func TestIconButton_SaysWhatItDoesWhenPointedAt(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	button := newIconButton(theme.CancelIcon(), "Clear the search", nil)
	_, layer := tipWindow(t, button)

	button.MouseIn(&desktop.MouseEvent{})
	require.Len(t, layer.Objects, 1, "resting on the button has to say what it is")
	require.Contains(t, labelTexts(layer), "Clear the search")

	button.MouseOut()
	require.Empty(t, layer.Objects, "and stop saying it on the way out")
}

// The icon-only buttons in the app all carry one.
func TestIconOnlyButtons_AllSayWhatTheyDo(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		state := newLibraryState()
		state.statuses[1] = updateStatus{Downloaded: true, HasUpdate: true, Diff: []string{"one"}}

		row := newGameRow().(*gameRow)
		bindGameRow(row, db.Game{ID: 1, Title: "One"}, rowBinding{sel: newGameSelection(), state: state})

		for what, ui := range map[string]fyne.CanvasObject{
			"the library":  lt.content,
			"a download":   DownloadsTabUI(test.NewWindow(nil), lt.dm),
			"a game's row": row,
		} {
			for _, button := range widgetsOfType[*widget.Button](ui) {
				require.NotEmpty(t, button.Text,
					"%s has a button with neither words nor a tip", what)
			}
			for _, button := range widgetsOfType[*iconButton](ui) {
				require.NotEmpty(t, button.tip, "%s has an icon button with nothing to say", what)
			}
		}
	})
}
