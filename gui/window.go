package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
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

	width := prefs.FloatWithFallback("windowWidth", 960)
	height := prefs.FloatWithFallback("windowHeight", 640)
	myWindow.Resize(fyne.NewSize(float32(width), float32(height)))

	myWindow.SetOnClosed(func() {
		size := myWindow.Canvas().Size()
		prefs.SetFloat("windowWidth", float64(size.Width))
		prefs.SetFloat("windowHeight", float64(size.Height))
	})

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

	myWindow.SetContent(mainTabs)
	mainTabs.SelectIndex(0) // Programmatically select the first tab to trigger OnSelected.

	myWindow.ShowAndRun()
}

func FileTabUI(win fyne.Window) fyne.CanvasObject {
	head := widget.NewLabelWithStyle("File Operations", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	hashTab := HashUI(win)
	sizeTab := SizeUI(win)
	fileTabs := container.NewAppTabs(
		container.NewTabItemWithIcon("File Hashes", theme.ContentAddIcon(), hashTab),
		container.NewTabItemWithIcon("Storage Size", theme.ViewFullScreenIcon(), sizeTab),
	)
	fileTabs.SetTabLocation(container.TabLocationTop)
	return container.NewBorder(
		head, nil, nil, nil,
		fileTabs,
	)
}
