package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
)

// The export formats are offered in a menu dropped from the Export button.
// Positioning it from the button's place inside its own container put the menu
// at the top of the window, hundreds of points from the button it belongs to.
func TestLibraryTab_ExportMenuDropsFromItsButton(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		// The menu is shown on the canvas of the window the library was built
		// against, so the library has to be what that window shows.
		lt, _, win := newLibraryFixtureInWindow(t, 3)
		win.SetContent(lt.content)
		win.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))

		export := buttonWithLabel(lt.content, "Export")
		require.NotNil(t, export, "the library has to offer an export")
		test.Tap(export)

		menu, ok := win.Canvas().Focused().(*widget.PopUpMenu)
		require.True(t, ok, "tapping Export has to open its menu")

		button := fyne.CurrentApp().Driver().AbsolutePositionForObject(export)
		require.InDelta(t, button.X, menu.Position().X, float64(theme.Padding()),
			"the menu has to line up with the button")
		require.GreaterOrEqual(t, menu.Position().Y+menu.Size().Height, button.Y,
			"the menu has to reach the button it dropped from")
		require.LessOrEqual(t, menu.Position().Y, button.Y+export.Size().Height,
			"and must not float below it")
	})
}
