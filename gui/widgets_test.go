package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
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
