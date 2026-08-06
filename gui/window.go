package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
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
	dm := NewDownloadManager()
	prefs := myApp.Preferences()

	state := loadWindowState(prefs)
	myWindow.Resize(fyne.NewSize(float32(state.Width), float32(state.Height)))

	content := buildMainContent(myWindow, version, authService, dm, loginer)

	registerShortcuts(myWindow.Canvas(), libraryShortcuts(
		func() {
			content.tabs.SelectIndex(0)
			myWindow.Canvas().Focus(content.library.searchEntry)
		},
		func() { content.library.refresh() },
	))

	// Remember where the user left the window.
	myWindow.SetOnClosed(func() {
		saveWindowState(prefs, windowStateOnClose(prefs, myWindow.Canvas().Size(),
			content.tabs.SelectedIndex(), content.library.split))
	})

	myWindow.SetContent(content.tabs)
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

	// The catalogue tab looks different when signed out, so it is built again
	// once the user logs in.
	var onLogin func()
	var settingsTab *container.TabItem
	content.library = LibraryTabUI(win, authService, dm, func() { onLogin() })

	catalogueTab := container.NewTabItemWithIcon(sectionCatalogue, theme.ListIcon(),
		content.library.content)

	// Signing in or out changes what the catalogue is, so it is built again,
	// and the one it replaces stops listening on its way out.
	rebuildLibrary := func() {
		content.library.close()
		content.library = LibraryTabUI(win, authService, dm, func() { onLogin() })
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
		settingsTab.Content = SettingsTabUI(win, onSignOut)
		settingsTab.Content.Refresh()
		content.tabs.SelectIndex(0)
	}

	settingsTab = container.NewTabItemWithIcon(sectionSettings, theme.SettingsIcon(),
		SettingsTabUI(win, func() { onSignOut() }))
	content.tabs = container.NewAppTabs(
		catalogueTab,
		container.NewTabItemWithIcon(sectionDownloads, theme.DownloadIcon(), DownloadsTabUI(dm)),
		container.NewTabItemWithIcon(sectionFileHashes, theme.DocumentIcon(), FileTabUI(win)),
		settingsTab,
		container.NewTabItemWithIcon(sectionAbout, theme.HelpIcon(), ShowAboutUI(version)),
	)
	content.tabs.SetTabLocation(container.TabLocationTop)
	content.tabs.OnSelected = func(tab *container.TabItem) {
		if tab.Text == sectionCatalogue {
			win.Canvas().Focus(content.library.searchEntry)
		}
	}

	return content
}

// selectRememberedTab opens the tab the user left gogg on, or the first one when
// that tab is no longer there. It is selected rather than left alone so that
// whatever the tab does on being opened happens for it too.
func selectRememberedTab(tabs *container.AppTabs, remembered int) {
	if remembered < 0 || remembered >= len(tabs.Items) {
		remembered = 0
	}
	tabs.SelectIndex(remembered)
}

// FileTabUI is the file tools tab. Storage size estimates moved to the library,
// where the games are selected. The tab names the section, so the pane does not
// repeat it.
func FileTabUI(win fyne.Window) fyne.CanvasObject {
	return HashUI(win)
}
