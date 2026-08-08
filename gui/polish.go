package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
)

const (
	prefNotifications   = "notificationsEnabled"
	prefWindowWidth     = "windowWidth"
	prefWindowHeight    = "windowHeight"
	prefSplitOffset     = "library.splitOffset"
	prefSelectedTabName = "window.selectedTabName"
)

// notify shows a desktop notification. It is a variable so tests can observe
// what the app would have shown.
var notify = func(title, content string) {
	fyne.CurrentApp().SendNotification(fyne.NewNotification(title, content))
}

// notifyBatchFinished tells the user their downloads are done, once: thirty
// games landing one by one is one piece of news, not thirty. Downloads outlast
// the user's attention, so this is how they find out without watching. title
// names the game when the whole batch was one.
func notifyBatchFinished(done, failed int, title string) {
	if !fyne.CurrentApp().Preferences().BoolWithFallback(prefNotifications, true) {
		return
	}
	switch {
	case done == 1 && failed == 0:
		notify("Download complete", fmt.Sprintf("%s finished downloading.", title))
	case done == 0:
		notify("Downloads failed", fmt.Sprintf("%d %s failed.", failed, downloadsWord(failed)))
	case failed > 0:
		notify("Downloads finished", fmt.Sprintf("%d %s downloaded, %d failed.",
			done, gamesWord(done), failed))
	default:
		notify("Downloads finished", fmt.Sprintf("%d %s downloaded.", done, gamesWord(done)))
	}
}

// libraryShortcut is a key combination and what it does.
type libraryShortcut struct {
	Shortcut *desktop.CustomShortcut
	Action   func()
}

// libraryShortcuts returns the keyboard shortcuts for the main window.
func libraryShortcuts(focusSearch, refreshCatalogue func()) []libraryShortcut {
	return []libraryShortcut{
		{
			Shortcut: &desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierControl},
			Action:   focusSearch,
		},
		{
			Shortcut: &desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl},
			Action:   refreshCatalogue,
		},
	}
}

// tabShortcuts puts each of the first count tabs on Ctrl+1 through Ctrl+9, so
// moving between them does not need the mouse.
func tabShortcuts(count int, selectTab func(int)) []libraryShortcut {
	keys := []fyne.KeyName{fyne.Key1, fyne.Key2, fyne.Key3, fyne.Key4, fyne.Key5,
		fyne.Key6, fyne.Key7, fyne.Key8, fyne.Key9}
	if count > len(keys) {
		count = len(keys)
	}

	shortcuts := make([]libraryShortcut, 0, count)
	for i := 0; i < count; i++ {
		index := i
		shortcuts = append(shortcuts, libraryShortcut{
			Shortcut: &desktop.CustomShortcut{KeyName: keys[i], Modifier: fyne.KeyModifierControl},
			Action:   func() { selectTab(index) },
		})
	}
	return shortcuts
}

// registerShortcuts wires the shortcuts onto a canvas.
func registerShortcuts(canvas fyne.Canvas, shortcuts []libraryShortcut) {
	for _, shortcut := range shortcuts {
		action := shortcut.Action
		canvas.AddShortcut(shortcut.Shortcut, func(fyne.Shortcut) {
			if action != nil {
				action()
			}
		})
	}
}

// windowState is what gogg remembers about the window between runs.
type windowState struct {
	Width, Height float64
	SplitOffset   float64
	// Tab is the name of the tab the window was left on. Names survive tabs
	// being added, moved, or retired, which indices did not.
	Tab string
}

const (
	defaultWindowWidth  = 960
	defaultWindowHeight = 640
	defaultSplitOffset  = 0.5
	// Neither pane may be squeezed out of sight, or the layout looks broken
	// with no obvious way back.
	minSplitOffset = 0.1
	maxSplitOffset = 0.9
)

func loadWindowState(prefs fyne.Preferences) windowState {
	offset := prefs.FloatWithFallback(prefSplitOffset, defaultSplitOffset)
	switch {
	case offset < minSplitOffset:
		offset = minSplitOffset
	case offset > maxSplitOffset:
		offset = maxSplitOffset
	}

	return windowState{
		Width:       prefs.FloatWithFallback(prefWindowWidth, defaultWindowWidth),
		Height:      prefs.FloatWithFallback(prefWindowHeight, defaultWindowHeight),
		SplitOffset: offset,
		Tab:         prefs.StringWithFallback(prefSelectedTabName, ""),
	}
}

func saveWindowState(prefs fyne.Preferences, state windowState) {
	prefs.SetFloat(prefWindowWidth, state.Width)
	prefs.SetFloat(prefWindowHeight, state.Height)
	prefs.SetFloat(prefSplitOffset, state.SplitOffset)
	prefs.SetString(prefSelectedTabName, state.Tab)
}

// windowStateOnClose is what to remember about the window. A session that never
// showed the library has no divider on screen to read, so the offset already
// stored is kept rather than replaced with the default.
func windowStateOnClose(prefs fyne.Preferences, size fyne.Size, tab string, split *container.Split) windowState {
	state := windowState{
		Width:       float64(size.Width),
		Height:      float64(size.Height),
		SplitOffset: loadWindowState(prefs).SplitOffset,
		Tab:         tab,
	}
	if split != nil {
		state.SplitOffset = split.Offset
	}
	return state
}
