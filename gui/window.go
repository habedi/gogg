package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
)

// Run starts the desktop GUI. loginer performs the GOG login flow when a
// signed-out user asks to log in.
func Run(version string, authService *auth.Service, loginer GogLoginer) {
	myApp := app.NewWithID("com.github.habedi.gogg")
	myApp.SetIcon(AppLogo)

	myApp.Settings().SetTheme(CreateThemeFromPreferences())

	myWindow := myApp.NewWindow("GOGG GUI")
	dm := NewDownloadManager()
	prefs := myApp.Preferences()

	state := loadWindowState(prefs)
	myWindow.Resize(fyne.NewSize(float32(state.Width), float32(state.Height)))

	// The catalogue tab looks different when signed out, so it is rebuilt once
	// the user logs in.
	var onLogin func()
	library := LibraryTabUI(myWindow, authService, dm, func() { onLogin() })

	catalogueTab := container.NewTabItemWithIcon("Catalogue", theme.ListIcon(), library.content)
	onLogin = func() {
		ShowLoginDialog(myWindow, loginer, func() {
			library = LibraryTabUI(myWindow, authService, dm, func() { onLogin() })
			catalogueTab.Content = library.content
			catalogueTab.Content.Refresh()
			myWindow.Canvas().Focus(library.searchEntry)
		})
	}

	mainTabs := container.NewAppTabs(
		catalogueTab,
		container.NewTabItemWithIcon("Downloads", theme.DownloadIcon(), DownloadsTabUI(dm)),
		container.NewTabItemWithIcon("File Ops", theme.DocumentIcon(), FileTabUI(myWindow)),
		container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), SettingsTabUI(myWindow)),
		container.NewTabItemWithIcon("About", theme.HelpIcon(), ShowAboutUI(version)),
	)

	mainTabs.OnSelected = func(tab *container.TabItem) {
		if tab.Text == "Catalogue" {
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
		size := myWindow.Canvas().Size()
		saved := windowState{
			Width:       float64(size.Width),
			Height:      float64(size.Height),
			SplitOffset: defaultSplitOffset,
			Tab:         mainTabs.SelectedIndex(),
		}
		if library.split != nil {
			saved.SplitOffset = library.split.Offset
		}
		saveWindowState(prefs, saved)
	})

	// The interface can be switched while the app is running, which means
	// building the one that was asked for from scratch.
	showInterface := func() {
		if prefs.BoolWithFallback(prefXMB, false) {
			myWindow.SetContent(newXMBShell(appCategories(myWindow, dm, version,
				func() fyne.CanvasObject { return library.content },
				func() { library.refresh() })))
			return
		}

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
// where the games are selected.
func FileTabUI(win fyne.Window) fyne.CanvasObject {
	head := widget.NewLabelWithStyle("File Hashes", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	return container.NewBorder(head, nil, nil, nil, HashUI(win))
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
			Title: "Catalogue", Icon: theme.ListIcon(),
			Items: []xmbItem{
				{Title: "Browse library", Detail: gameCountDetail, Pane: catalogue},
				{Title: "Refresh catalogue", Action: refresh},
			},
		},
		{
			Title: "Downloads", Icon: theme.DownloadIcon(),
			Items: []xmbItem{{
				Title:  "Active downloads",
				Detail: func() string { return downloadCountDetail(dm) },
				Pane:   func() fyne.CanvasObject { return DownloadsTabUI(dm) },
			}},
		},
		{
			Title: "File Ops", Icon: theme.DocumentIcon(),
			Items: []xmbItem{{
				Title: "File hashes",
				Pane:  func() fyne.CanvasObject { return FileTabUI(win) },
			}},
		},
		{
			Title: "Settings", Icon: theme.SettingsIcon(),
			Items: []xmbItem{{
				Title: "Preferences",
				Pane:  func() fyne.CanvasObject { return SettingsTabUI(win) },
			}},
		},
		{
			Title: "About", Icon: theme.HelpIcon(),
			Items: []xmbItem{{
				Title: "About gogg",
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
