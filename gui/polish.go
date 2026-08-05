package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
)

const (
	prefNotifications = "notificationsEnabled"
	prefWindowWidth   = "windowWidth"
	prefWindowHeight  = "windowHeight"
	prefSplitOffset   = "library.splitOffset"
	prefSelectedTab   = "window.selectedTab"
)

// notify shows a desktop notification. It is a variable so tests can observe
// what the app would have shown.
var notify = func(title, content string) {
	fyne.CurrentApp().SendNotification(fyne.NewNotification(title, content))
}

// notifyDownloadFinished tells the user a download is done. Downloads outlast
// the user's attention, so this is how they find out without watching.
func notifyDownloadFinished(gameTitle string) {
	if !fyne.CurrentApp().Preferences().BoolWithFallback(prefNotifications, true) {
		return
	}
	notify("Download complete", fmt.Sprintf("%s finished downloading.", gameTitle))
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
	Tab           int
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
		Tab:         prefs.IntWithFallback(prefSelectedTab, 0),
	}
}

func saveWindowState(prefs fyne.Preferences, state windowState) {
	prefs.SetFloat(prefWindowWidth, state.Width)
	prefs.SetFloat(prefWindowHeight, state.Height)
	prefs.SetFloat(prefSplitOffset, state.SplitOffset)
	prefs.SetInt(prefSelectedTab, state.Tab)
}

// windowStateOnClose is what to remember about the window. A session that never
// showed the library has no divider on screen to read, so the offset already
// stored is kept rather than replaced with the default.
func windowStateOnClose(prefs fyne.Preferences, size fyne.Size, tab int, split *container.Split) windowState {
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

// showingTabs reports whether the tab interface is what the window is showing.
// The tabs are built either way, so a shortcut that moves through them has to
// ask first: in the cross interface it would take the focus to a box that is
// not on screen.
func showingTabs(win fyne.Window, tabs fyne.CanvasObject) bool {
	return win != nil && win.Content() == tabs
}

// handOverCatalogue gives the catalogue widgets to the interface that is about
// to show them. The tabs are built whether or not they are on screen, and the
// same widgets cannot hang in two places at once.
func handOverCatalogue(tab *container.TabItem, catalogue fyne.CanvasObject, toTabs bool) {
	if toTabs {
		tab.Content = catalogue
		return
	}
	tab.Content = container.NewStack()
}
