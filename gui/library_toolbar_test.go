package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
	"github.com/stretchr/testify/require"
)

// Sorting and exporting wait in a menu dropped from the toolbar's last button.
// Positioning it from the button's place inside its own container put the menu
// at the top of the window, hundreds of points from the button it belongs to.
func TestLibraryTab_MoreMenuDropsFromItsButton(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		// The menu is shown on the canvas of the window the library was built
		// against, so the library has to be what that window shows.
		lt, _, win := newLibraryFixtureInWindow(t, 3)
		win.SetContent(lt.content)
		win.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))

		more := iconButtonWithTip(lt.content, tipMore)
		require.NotNil(t, more, "the library has to offer sorting and exporting")
		test.Tap(more)

		menu, ok := win.Canvas().Focused().(*widget.PopUpMenu)
		require.True(t, ok, "tapping the button has to open its menu")

		labels := make([]string, 0, len(lt.moreMenu().Items))
		for _, item := range lt.moreMenu().Items {
			labels = append(labels, item.Label)
		}
		require.Contains(t, labels, "Export Game List as CSV")
		require.Contains(t, labels, "Export Full Catalogue as JSON")

		button := fyne.CurrentApp().Driver().AbsolutePositionForObject(more)
		require.InDelta(t, button.X, menu.Position().X, float64(theme.Padding()),
			"the menu has to line up with the button")
		require.GreaterOrEqual(t, menu.Position().Y+menu.Size().Height, button.Y,
			"the menu has to reach the button it dropped from")
		require.LessOrEqual(t, menu.Position().Y, button.Y+more.Size().Height,
			"and must not float below it")
	})
}

// refreshRecorder stands in for the catalogue refresh, so a test can press the
// button without reaching GOG and decide when the refresh finishes.
type refreshRecorder struct {
	started  int
	onFinish func()
}

func captureRefreshes(t *testing.T) *refreshRecorder {
	t.Helper()
	recorder := &refreshRecorder{}
	original := refreshCatalogue
	refreshCatalogue = func(_ fyne.Window, _ *auth.Service, onFinish func()) {
		recorder.started++
		recorder.onFinish = onFinish
	}
	t.Cleanup(func() { refreshCatalogue = original })
	return recorder
}

func (r *refreshRecorder) finish() {
	if r.onFinish != nil {
		r.onFinish()
	}
}

// Refreshing must not throw away what the user was looking for. The search box
// was emptied, taking the filter, and the chosen collection with it.
func TestLibraryTab_RefreshKeepsTheSearch(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		refreshes := captureRefreshes(t)
		lt, _ := newLibraryFixture(t, 3)
		lt.searchEntry.SetText("Game 1")

		test.Tap(iconButtonWithTip(lt.content, tipRefresh))
		require.Equal(t, "Game 1", lt.searchEntry.Text, "the search has to survive the refresh")

		refreshes.finish()
		require.Equal(t, "Game 1", lt.searchEntry.Text, "and survive it finishing")
		require.Len(t, lt.listed(), 1, "the list stays filtered by what is in the box")
	})
}

// The empty-library placeholder starts the same refresh as the toolbar button,
// so pressing it twice must not start two.
func TestLibraryTab_EmptyLibraryRefreshesOnceAtATime(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		refreshes := captureRefreshes(t)
		lt, _ := newLibraryFixture(t, 0)

		button := buttonWithLabel(lt.content, "Refresh Catalogue")
		require.NotNil(t, button, "an empty library has to offer a refresh")

		test.Tap(button)
		test.Tap(button)
		require.Equal(t, 1, refreshes.started, "a second press must not start a second refresh")
		require.True(t, button.Disabled(), "the button has to show a refresh is running")

		refreshes.finish()
		require.False(t, button.Disabled(), "and be usable again once it is done")
	})
}

// The toggles name what choosing them will do, not the state they are in: one
// naming the state and its neighbour naming the action reads as a
// contradiction.
func TestLibraryTab_TheTogglesNameWhatTheyDo(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)

		require.NotNil(t, iconButtonWithTip(lt.content, tipShowList),
			"showing the covers, the button offers the list")

		sort := lt.moreMenu().Items[0]
		require.Equal(t, "Sort Z-A", sort.Label, "sorted A to Z, the menu offers Z to A")
		sort.Action()

		require.Equal(t, "Sort A-Z", lt.moreMenu().Items[0].Label, "and back again")
		require.Equal(t, "Game 3", lt.listed()[0].Title, "and the list follows the order")
	})
}

// The toolbar's icon buttons each say what they do on hover; Filters keeps its
// word because it is the one whose meaning no icon carries.
func TestLibraryTab_ToolbarOffersEveryTool(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)

		for _, tip := range []string{tipCollections, tipRefresh, tipShowList, tipMore} {
			require.NotNil(t, iconButtonWithTip(lt.content, tip), "no button says %q", tip)
		}
		require.NotNil(t, buttonWithLabel(lt.content, "Filters"))
	})
}

// The toolbar has one job, and holding a second set of settings was not it.
func TestLibraryTab_ToolbarNoLongerHoldsSettings(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		require.Nil(t, buttonWithLabel(lt.content, "Update Settings"),
			"update detection is set in Settings now")
	})
}

// Changing how updates are detected has to make the library work them out
// again, wherever the change was made.
func TestSettings_ChangingUpdateDetectionRechecksTheLibrary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, win := newLibraryFixtureShown(t, 2)
		held := holdStatusWork(t)

		settings := SettingsTabUI(win, func() {})
		check := checkWithLabel(settings, "Include patches")
		require.NotNil(t, check)
		check.SetChecked(true)

		require.Contains(t, labelTexts(lt.content), "Checking downloads...",
			"the library has to look again")
		held.run()
		require.NotContains(t, labelTexts(lt.content), "Checking downloads...")
	})
}
