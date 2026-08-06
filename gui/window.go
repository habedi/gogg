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

	// The catalogue tab looks different when signed out, so it is rebuilt once
	// the user logs in.
	var onLogin func()
	var mainTabs *container.AppTabs
	var settingsTab *container.TabItem
	library := LibraryTabUI(myWindow, authService, dm, func() { onLogin() })

	catalogueTab := container.NewTabItemWithIcon(sectionCatalogue, theme.ListIcon(), library.content)

	// Signing in or out changes what the catalogue is, so it is built again,
	// and the one it replaces stops listening on its way out.
	rebuildLibrary := func() {
		library.close()
		library = LibraryTabUI(myWindow, authService, dm, func() { onLogin() })
		catalogueTab.Content = library.content
		catalogueTab.Content.Refresh()
	}
	onLogin = func() {
		ShowLoginDialog(myWindow, loginer, func() {
			rebuildLibrary()
			myWindow.Canvas().Focus(library.searchEntry)
		})
	}
	// Signing out also takes the way out of the settings with it, so they are
	// built again too.
	var onSignOut func()
	onSignOut = func() {
		rebuildLibrary()
		settingsTab.Content = SettingsTabUI(myWindow, onSignOut)
		settingsTab.Content.Refresh()
		mainTabs.SelectIndex(0)
	}

	settingsTab = container.NewTabItemWithIcon(sectionSettings, theme.SettingsIcon(),
		SettingsTabUI(myWindow, func() { onSignOut() }))
	mainTabs = container.NewAppTabs(
		catalogueTab,
		container.NewTabItemWithIcon(sectionDownloads, theme.DownloadIcon(), DownloadsTabUI(dm)),
		container.NewTabItemWithIcon(sectionFileHashes, theme.DocumentIcon(), FileTabUI(myWindow)),
		settingsTab,
		container.NewTabItemWithIcon(sectionAbout, theme.HelpIcon(), ShowAboutUI(version)),
	)

	mainTabs.OnSelected = func(tab *container.TabItem) {
		if tab.Text == sectionCatalogue {
			myWindow.Canvas().Focus(library.searchEntry)
		}
	}

	mainTabs.SetTabLocation(container.TabLocationTop)

	registerShortcuts(myWindow.Canvas(), libraryShortcuts(
		func() {
			mainTabs.SelectIndex(0)
			myWindow.Canvas().Focus(library.searchEntry)
		},
		func() { library.refresh() },
	))

	// Remember where the user left the window.
	myWindow.SetOnClosed(func() {
		saveWindowState(prefs, windowStateOnClose(prefs, myWindow.Canvas().Size(),
			mainTabs.SelectedIndex(), library.split))
	})

	myWindow.SetContent(mainTabs)
	// Select a tab explicitly so OnSelected runs for it.
	if state.Tab >= 0 && state.Tab < len(mainTabs.Items) {
		mainTabs.SelectIndex(state.Tab)
	} else {
		mainTabs.SelectIndex(0)
	}

	myWindow.ShowAndRun()
}

// FileTabUI is the file tools tab. Storage size estimates moved to the library,
// where the games are selected. The tab names the section, so the pane does not
// repeat it.
func FileTabUI(win fyne.Window) fyne.CanvasObject {
	return HashUI(win)
}
