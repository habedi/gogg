package gui

import (
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// --- notifications ---

// notificationRecorder collects what the app would have shown. Downloads
// notify from their own goroutine, so it is guarded.
type notificationRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *notificationRecorder) add(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, text)
}

func (r *notificationRecorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

func captureNotifications(t *testing.T) *notificationRecorder {
	t.Helper()
	recorder := &notificationRecorder{}
	original := notify
	notify = func(title, content string) { recorder.add(title + ": " + content) }
	t.Cleanup(func() { notify = original })
	return recorder
}

// A download finishes long after you have looked away from the window.
func TestNotifyDownloadFinished(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	recorder := captureNotifications(t)

	notifyDownloadFinished("Baldur's Gate")

	require.Len(t, recorder.all(), 1)
	require.Contains(t, recorder.all()[0], "Baldur's Gate")
}

func TestNotifyDownloadFinished_RespectsThePreference(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Preferences().SetBool(prefNotifications, false)
	recorder := captureNotifications(t)

	notifyDownloadFinished("Baldur's Gate")

	require.Empty(t, recorder.all())
}

func TestExecuteDownload_NotifiesWhenItFinishes(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Preferences().SetBool("soundEnabled", false)
	recorder := captureNotifications(t)

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	require.NoError(t, executeDownload(dm, queuedDownload{
		authService: stubAuthService(), game: db.Game{ID: 11, Title: "Finished Game", Data: "{}"},
		downloadPath: t.TempDir(), language: "English", platformName: "windows", numThreads: 1,
	}))

	require.Eventually(t, func() bool { return len(recorder.all()) > 0 }, 5*time.Second, 10*time.Millisecond)
	require.Contains(t, recorder.all()[0], "Finished Game")

	// Let the download goroutine finish before the stub is put back.
	require.Eventually(t, func() bool {
		activeDownloadsMutex.Lock()
		defer activeDownloadsMutex.Unlock()
		_, stillRunning := activeDownloads[11]
		return !stillRunning
	}, 5*time.Second, 10*time.Millisecond)
}

// --- keyboard shortcuts ---

func TestLibraryShortcuts(t *testing.T) {
	focused, refreshed := 0, 0
	shortcuts := libraryShortcuts(func() { focused++ }, func() { refreshed++ })

	require.NotEmpty(t, shortcuts)
	for _, s := range shortcuts {
		require.NotNil(t, s.Shortcut, "every shortcut needs a key combination")
		require.NotNil(t, s.Action)
	}

	shortcutNamed(t, shortcuts, fyne.KeyF).Action()
	require.Equal(t, 1, focused, "Ctrl+F jumps to the search box")

	shortcutNamed(t, shortcuts, fyne.KeyR).Action()
	require.Equal(t, 1, refreshed, "Ctrl+R refreshes the catalogue")
}

func shortcutNamed(t *testing.T, shortcuts []libraryShortcut, key fyne.KeyName) libraryShortcut {
	t.Helper()
	for _, s := range shortcuts {
		if s.Shortcut.KeyName == key {
			return s
		}
	}
	t.Fatalf("no shortcut bound to %v", key)
	return libraryShortcut{}
}

// --- window state ---

func TestWindowState_RoundTrips(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	prefs := app.Preferences()

	saveWindowState(prefs, windowState{Width: 1200, Height: 800, SplitOffset: 0.35, Tab: 2})

	restored := loadWindowState(prefs)
	require.Equal(t, float64(1200), restored.Width)
	require.Equal(t, float64(800), restored.Height)
	require.InDelta(t, 0.35, restored.SplitOffset, 0.001)
	require.Equal(t, 2, restored.Tab)
}

func TestWindowState_Defaults(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	state := loadWindowState(app.Preferences())
	require.Equal(t, float64(960), state.Width)
	require.Equal(t, float64(640), state.Height)
	require.InDelta(t, 0.5, state.SplitOffset, 0.001)
	require.Zero(t, state.Tab)
}

// A saved offset of 0 or 1 would open the app with one pane invisible and no
// obvious way back.
func TestWindowState_KeepsBothPanesReachable(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	prefs := app.Preferences()

	saveWindowState(prefs, windowState{Width: 960, Height: 640, SplitOffset: 0})
	require.GreaterOrEqual(t, loadWindowState(prefs).SplitOffset, 0.1)

	saveWindowState(prefs, windowState{Width: 960, Height: 640, SplitOffset: 1})
	require.LessOrEqual(t, loadWindowState(prefs).SplitOffset, 0.9)
}

func TestLibraryTab_RestoresTheSavedSplit(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	saveWindowState(app.Preferences(), windowState{Width: 960, Height: 640, SplitOffset: 0.3})

	lt, _ := newLibraryFixture(t, 2)
	require.NotNil(t, lt.split)
	require.InDelta(t, 0.3, lt.split.Offset, 0.001)
}
