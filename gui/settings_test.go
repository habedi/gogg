package gui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
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

	ui := SettingsTabUI(win, func() {})
	maxConcurrentSelect(t, ui).SetSelected("5")

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	require.Equal(t, 5, dm.maxConcurrent(), "the queue must use the chosen limit")

	// The choice also has to survive a rebuild of the settings tab.
	require.Equal(t, "5", maxConcurrentSelect(t, SettingsTabUI(win, func() {})).Selected)
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

	_ = SettingsTabUI(win, func() {})

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

	_ = SettingsTabUI(win, func() {})

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

	ui := SettingsTabUI(win, func() {})
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

// A speed limit that is not a number was ignored in silence, leaving the last
// one in force while the box showed something else.
func TestSettings_MarksASpeedLimitItCannotRead(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	defer win.Close()

	ui := SettingsTabUI(win, func() {})
	speed := speedLimitEntry(t, ui)

	speed.SetText("as fast as it goes")
	require.Error(t, speed.Validate(), "a limit that is not a number has to be marked")

	speed.SetText("500")
	require.NoError(t, speed.Validate())

	speed.SetText("")
	require.NoError(t, speed.Validate(), "an empty box means no limit, which is not a mistake")
}

// The unit belongs on the field, not only in the placeholder that disappears as
// soon as a number is typed.
func TestSettings_SpeedLimitSaysItsUnit(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	defer win.Close()

	require.Contains(t, formItemLabels(SettingsTabUI(win, func() {})), "Speed Limit (KB/s)")
}

func speedLimitEntry(t *testing.T, ui fyne.CanvasObject) *widget.Entry {
	t.Helper()
	entries := widgetsOfType[*widget.Entry](ui)
	require.Len(t, entries, 1, "the settings have one box to type in, the speed limit")
	return entries[0]
}

// formItemLabels is what the forms in a tree call their fields.
func formItemLabels(root fyne.CanvasObject) []string {
	var texts []string
	for _, form := range widgetsOfType[*widget.Form](root) {
		for _, item := range form.Items {
			texts = append(texts, item.Text)
		}
	}
	return texts
}

// gogg could be signed in to but never out of.
func TestSettings_SignsOut(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })
	require.NoError(t, db.UpsertTokenRecord(&db.Token{
		AccessToken: "a", RefreshToken: "r", ExpiresAt: "2999-01-01T00:00:00Z",
	}))

	signedOut := 0
	ui := SettingsTabUI(win, func() { signedOut++ })
	win.SetContent(ui)

	logout := buttonWithLabel(ui, "Log Out")
	require.NotNil(t, logout, "a signed-in gogg has to offer a way out")

	test.Tap(logout)
	confirm := buttonWithLabel(topOverlay(t), "Yes")
	require.NotNil(t, confirm, "signing out has to be asked about first")
	token, err := db.GetTokenRecord()
	require.NoError(t, err)
	require.NotNil(t, token, "and nothing happens until it is answered")

	test.Tap(confirm)

	token, err = db.GetTokenRecord()
	require.NoError(t, err)
	require.Nil(t, token, "signing out clears what gogg was signed in with")
	require.Equal(t, 1, signedOut, "and the rest of the app is told")
}

// Options that apply to every download belong with the settings, not behind a
// button in the library toolbar.
func TestSettings_CarriesTheUpdateDetectionOptions(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	defer win.Close()

	ui := SettingsTabUI(win, func() {})

	var labels []string
	for _, check := range widgetsOfType[*widget.Check](ui) {
		labels = append(labels, check.Text)
	}
	require.Subset(t, labels, []string{
		"Include extras in update check", "Include DLCs in update check",
		"Include patches", "Scan folders when history missing",
	})
}
