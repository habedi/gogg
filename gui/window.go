package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
)

// appName is what gogg calls itself, wherever it says so.
const appName = "Gogg"

// The sections of the app, named once so the tabs and the cross interface
// cannot drift apart.
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
	library := LibraryTabUI(myWindow, authService, dm, func() { onLogin() })

	catalogueTab := container.NewTabItemWithIcon(sectionCatalogue, theme.ListIcon(), library.content)
	onLogin = func() {
		ShowLoginDialog(myWindow, loginer, func() {
			// The signed-out library is replaced, so it stops following the
			// catalogue on its way out.
			library.close()
			library = LibraryTabUI(myWindow, authService, dm, func() { onLogin() })
			handOverCatalogue(catalogueTab, library.content, showingTabs(myWindow, mainTabs))
			catalogueTab.Content.Refresh()
			myWindow.Canvas().Focus(library.searchEntry)
		})
	}

	mainTabs = container.NewAppTabs(
		catalogueTab,
		container.NewTabItemWithIcon(sectionDownloads, theme.DownloadIcon(), DownloadsTabUI(dm)),
		container.NewTabItemWithIcon(sectionFileHashes, theme.DocumentIcon(), FileTabUI(myWindow)),
		container.NewTabItemWithIcon(sectionSettings, theme.SettingsIcon(), SettingsTabUI(myWindow)),
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
			// The cross interface has no search box on screen to focus.
			if !showingTabs(myWindow, mainTabs) {
				return
			}
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

	// The interface can be switched while the app is running, which means
	// building the one that was asked for from scratch.
	showInterface := func() {
		if prefs.BoolWithFallback(prefXMB, false) {
			handOverCatalogue(catalogueTab, library.content, false)
			myWindow.SetContent(newXMBShell(appCategories(myWindow, dm, version,
				func() fyne.CanvasObject { return library.content },
				func() { library.refresh() })))
			return
		}

		handOverCatalogue(catalogueTab, library.content, true)
		myWindow.SetContent(mainTabs)
		// Select a tab explicitly so OnSelected runs for it.
		if state.Tab >= 0 && state.Tab < len(mainTabs.Items) {
			mainTabs.SelectIndex(state.Tab)
		} else {
			mainTabs.SelectIndex(0)
		}
	}
	onInterfaceChanged = showInterface
	showInterface()

	myWindow.ShowAndRun()
}

// FileTabUI is the file tools tab. Storage size estimates moved to the library,
// where the games are selected. The tab names the section, so the pane does not
// repeat it.
func FileTabUI(win fyne.Window) fyne.CanvasObject {
	return HashUI(win)
}

// onInterfaceChanged is set by Run so that choosing the other interface in the
// settings takes effect at once. It is nil in tests, where there is no window to
// rebuild.
var onInterfaceChanged func()

// appCategories describes gogg for the cross interface: the same sections as the
// tabs, with what can be done in each listed under it. catalogue is read when an
// item is opened, because the library is rebuilt when the user logs in.
func appCategories(win fyne.Window, dm *DownloadManager, version string,
	catalogue func() fyne.CanvasObject, refresh func(),
) []xmbCategory {
	return []xmbCategory{
		{
			Title: sectionCatalogue, Icon: theme.ListIcon(),
			Items: []xmbItem{
				{Title: "Browse Library", Detail: gameCountDetail, Pane: catalogue},
				{Title: "Refresh Catalogue", Action: refresh},
			},
		},
		{
			Title: sectionDownloads, Icon: theme.DownloadIcon(),
			Items: []xmbItem{{
				Title:  "Active Downloads",
				Detail: func() string { return downloadCountDetail(dm) },
				Pane:   func() fyne.CanvasObject { return DownloadsTabUI(dm) },
			}},
		},
		{
			Title: sectionFileHashes, Icon: theme.DocumentIcon(),
			Items: []xmbItem{{
				Title: "Hash Files",
				Pane:  func() fyne.CanvasObject { return FileTabUI(win) },
			}},
		},
		{
			Title: sectionSettings, Icon: theme.SettingsIcon(),
			Items: []xmbItem{{
				Title: "Preferences",
				Pane:  func() fyne.CanvasObject { return SettingsTabUI(win) },
			}},
		},
		{
			Title: sectionAbout, Icon: theme.HelpIcon(),
			Items: []xmbItem{{
				Title: "About " + appName,
				Pane:  func() fyne.CanvasObject { return ShowAboutUI(version) },
			}},
		},
	}
}

// gameCountDetail says how much is in the catalogue, so the bar carries the
// number without opening anything.
func gameCountDetail() string {
	games, err := db.GetCatalogue()
	if err != nil || len(games) == 0 {
		return ""
	}
	return fmt.Sprintf("%d games", len(games))
}

// downloadCountDetail says how many downloads are on the go.
func downloadCountDetail(dm *DownloadManager) string {
	if dm == nil {
		return ""
	}
	tasks, err := dm.Tasks.Get()
	if err != nil || len(tasks) == 0 {
		return ""
	}
	return fmt.Sprintf("%d in the queue", len(tasks))
}
