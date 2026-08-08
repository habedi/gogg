package gui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"github.com/habedi/gogg/auth"
)

// appName is what gogg calls itself, wherever it says so.
const appName = "Gogg"

// The sections of the app, named once so a tab and what it holds cannot drift
// apart.
const (
	sectionCatalogue  = "Catalogue"
	sectionDownloads  = "Downloads"
	sectionFileHashes = "File Hashes"
	sectionSettings   = "Settings"
	sectionAbout      = "About"
)

// Run starts the desktop GUI. loginer performs the GOG login flow when a
// signed-out user asks to log in.
func Run(version string, authService *auth.Service, loginer GogLoginer) {
	myApp := app.NewWithID("com.github.habedi.gogg")
	myApp.SetIcon(AppLogo)

	myApp.Settings().SetTheme(CreateThemeFromPreferences())

	myWindow := myApp.NewWindow(appName)
	dm := NewDownloadManager(authService)
	prefs := myApp.Preferences()

	state := loadWindowState(prefs)
	myWindow.Resize(fyne.NewSize(float32(state.Width), float32(state.Height)))

	content := buildMainContent(myWindow, version, authService, dm, loginer)

	shortcuts := libraryShortcuts(
		func() {
			content.tabs.SelectIndex(0)
			myWindow.Canvas().Focus(content.library.searchEntry)
		},
		func() { content.library.refresh() },
	)
	shortcuts = append(shortcuts, tabShortcuts(len(content.tabs.Items), content.tabs.SelectIndex)...)
	registerShortcuts(myWindow.Canvas(), shortcuts)

	// Remember where the user left the window.
	myWindow.SetOnClosed(func() {
		saveWindowState(prefs, windowStateOnClose(prefs, myWindow.Canvas().Size(),
			content.tabs.Selected().Text, content.library.split))
	})

	// Closing with downloads still running would stop them, so it is asked
	// about rather than done. Their bytes and a Resume survive either way,
	// but a window that vanishes mid download surprises people.
	myWindow.SetCloseIntercept(func() {
		if n := dm.inFlightCount(); n > 0 {
			dialog.ShowConfirm("Quit gogg?",
				fmt.Sprintf("%d %s still running. They will stop, and you can resume them next time. Quit anyway?",
					n, downloadsWord(n)),
				func(confirmed bool) {
					if confirmed {
						dm.PersistHistory()
						myWindow.Close()
					}
				}, myWindow)
			return
		}
		myWindow.Close()
	})

	myWindow.SetContent(installTipLayer(myWindow.Canvas(), content.tabs))
	selectRememberedTab(content.tabs, state.Tab)

	myWindow.ShowAndRun()
}

// mainWindowContent is what the window shows: the tabs, and the library that is
// in them now. The library is built again when the user signs in or out, so it
// is read from here rather than captured.
type mainWindowContent struct {
	tabs    *container.AppTabs
	library *libraryTab
}

// buildMainContent assembles the window. Run shows what it returns; a test can
// build it and walk it, which is the only way to find the parts that are wrong
// together while each is right on its own.
func buildMainContent(win fyne.Window, version string, authService *auth.Service,
	dm *DownloadManager, loginer GogLoginer,
) *mainWindowContent {
	content := &mainWindowContent{}
	st := openStores()

	// The catalogue tab looks different when signed out, so it is built again
	// once the user logs in.
	var onLogin func()
	var settingsTab *container.TabItem
	content.library = LibraryTabUI(win, authService, dm, st, func() { onLogin() })

	catalogueTab := container.NewTabItemWithIcon(sectionCatalogue, theme.ListIcon(),
		content.library.content)

	// Signing in or out changes what the catalogue is, so it is built again,
	// and the one it replaces stops listening on its way out.
	rebuildLibrary := func() {
		content.library.close()
		content.library = LibraryTabUI(win, authService, dm, st, func() { onLogin() })
		catalogueTab.Content = content.library.content
		catalogueTab.Content.Refresh()
	}
	onLogin = func() {
		ShowLoginDialog(win, loginer, func() {
			rebuildLibrary()
			win.Canvas().Focus(content.library.searchEntry)
		})
	}
	// Signing out takes the way out of the settings with it, so they are built
	// again too.
	var onSignOut func()
	onSignOut = func() {
		rebuildLibrary()
		settingsTab.Content = SettingsTabUI(win, st, onSignOut)
		settingsTab.Content.Refresh()
		content.tabs.SelectIndex(0)
	}

	settingsTab = container.NewTabItemWithIcon(sectionSettings, theme.SettingsIcon(),
		SettingsTabUI(win, st, func() { onSignOut() }))
	downloadsTab := container.NewTabItemWithIcon(sectionDownloads, theme.DownloadIcon(),
		DownloadsTabUI(win, dm))
	// File hashes are no longer a tab of their own: downloads verify
	// themselves now, so manual hashing is a utility behind the library's
	// More menu rather than a top-level destination.
	content.tabs = container.NewAppTabs(
		catalogueTab,
		downloadsTab,
		settingsTab,
		container.NewTabItemWithIcon(sectionAbout, theme.HelpIcon(), ShowAboutUI(version)),
	)
	content.tabs.SetTabLocation(container.TabLocationTop)
	content.tabs.OnSelected = func(tab *container.TabItem) {
		if tab == catalogueTab {
			win.Canvas().Focus(content.library.searchEntry)
		}
	}

	// The tab says how many downloads are on their way, so someone browsing the
	// catalogue does not have to open it to know something is happening.
	retitleDownloads := func() {
		title := sectionDownloads
		if n := dm.inFlightCount(); n > 0 {
			title = fmt.Sprintf("%s (%d)", sectionDownloads, n)
		}
		if downloadsTab.Text != title {
			downloadsTab.Text = title
			content.tabs.Refresh()
		}
	}
	countDownloads := binding.NewDataListener(retitleDownloads)
	dm.Tasks.AddListener(countDownloads)
	dm.states().AddListener(countDownloads)

	return content
}

// selectRememberedTab opens the tab the user left gogg on, or the first one
// when that tab is no longer there. Tabs are remembered by name, so a tab
// that moves or retires cannot misdirect the ones after it. It is selected
// rather than left alone so that whatever the tab does on being opened
// happens for it too.
func selectRememberedTab(tabs *container.AppTabs, remembered string) {
	for index, item := range tabs.Items {
		// A prefix match, because the downloads tab counts its work in its
		// name ("Downloads (2)") and may have been saved mid-download.
		if item.Text == remembered || (item.Text != "" && strings.HasPrefix(remembered, item.Text+" (")) {
			tabs.SelectIndex(index)
			return
		}
	}
	tabs.SelectIndex(0)
}

// FileTabUI is the file tools tab. Storage size estimates moved to the library,
// where the games are selected. The tab names the section, so the pane does not
// repeat it.
func FileTabUI(win fyne.Window) fyne.CanvasObject {
	return HashUI(win)
}
