package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/stretchr/testify/require"
)

func maxConcurrentSelect(t *testing.T, ui fyne.CanvasObject) *widget.Select {
	t.Helper()
	for _, s := range widgetsOfType[*widget.Select](ui) {
		if len(s.Options) == 10 && s.Options[0] == "1" && s.Options[9] == "10" {
			return s
		}
	}
	t.Fatal("could not find the Max Concurrent select")
	return nil
}

// Choosing a concurrent download limit in Settings has to change how many
// downloads the queue actually runs.
func TestSettings_MaxConcurrentTakesEffect(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	defer win.Close()

	ui := SettingsTabUI(win)
	maxConcurrentSelect(t, ui).SetSelected("5")

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	require.Equal(t, 5, dm.maxConcurrent(), "the queue must use the chosen limit")

	// The choice also has to survive a rebuild of the settings tab.
	require.Equal(t, "5", maxConcurrentSelect(t, SettingsTabUI(win)).Selected)
}

// Older versions stored this preference as a string; that choice must still be
// honoured after upgrading rather than silently reverting to the default.
func TestSettings_MaxConcurrentAcceptsLegacyStringValue(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	app.Preferences().SetString("download.maxConcurrent", "7")

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	require.Equal(t, 7, dm.maxConcurrent())
}

func TestSettings_MaxConcurrentFallsBackToDefault(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	require.Equal(t, 2, dm.maxConcurrent())

	app.Preferences().SetString("download.maxConcurrent", "not a number")
	require.Equal(t, 2, dm.maxConcurrent())
}

// A speed limit saved in an earlier session has to be in force from startup,
// not only after the field is edited again.
func TestSettings_SavedSpeedLimitIsApplied(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	defer win.Close()

	client.SetGlobalDownloadRateLimit(0)
	t.Cleanup(func() { client.SetGlobalDownloadRateLimit(0) })
	app.Preferences().SetInt("download.maxSpeedKBps", 512)

	_ = SettingsTabUI(win)

	require.NotNil(t, client.GlobalDownloadRateLimiter,
		"the saved speed limit must be applied without the user touching the field")
}

func TestSettings_NoSavedSpeedLimitLeavesDownloadsUnthrottled(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	defer win.Close()

	client.SetGlobalDownloadRateLimit(0)
	t.Cleanup(func() { client.SetGlobalDownloadRateLimit(0) })
	app.Preferences().SetInt("download.maxSpeedKBps", 0)

	_ = SettingsTabUI(win)

	require.Nil(t, client.GlobalDownloadRateLimiter)
}

// The settings are taller than the window gogg opens at. Centred and fixed in
// place, the download limits sat below the bottom edge with no way to reach
// them, so the tab has to scroll.
func TestSettings_ScrollSoEveryOptionCanBeReached(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	defer win.Close()

	ui := SettingsTabUI(win)
	win.SetContent(ui)
	win.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))

	scrolls := widgetsOfType[*container.Scroll](ui)
	require.NotEmpty(t, scrolls, "settings taller than the window have to scroll")

	body := scrolls[0]
	require.Greater(t, body.Content.MinSize().Height, win.Canvas().Size().Height,
		"this test only says something while the settings are taller than the window")

	body.ScrollToBottom()
	limit := maxConcurrentSelect(t, ui)
	bottom := fyne.CurrentApp().Driver().AbsolutePositionForObject(limit).Y + limit.Size().Height
	require.Greater(t, bottom, float32(0), "the last setting has to be laid out")
	require.LessOrEqual(t, bottom, win.Canvas().Size().Height,
		"the last setting has to come into view once scrolled to")
}
