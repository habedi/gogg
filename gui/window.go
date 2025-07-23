package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
)

func Run(version string, authService *auth.Service) {
	myApp := app.NewWithID("com.habedi.gogg")

	themePref := myApp.Preferences().StringWithFallback("theme", "light")
	if themePref == "dark" {
		myApp.Settings().SetTheme(NewCustomDarkTheme())
	} else {
		myApp.Settings().SetTheme(theme.LightTheme())
	}

	myWindow := myApp.NewWindow("GOGG GUI")
	dm := NewDownloadManager()

	mainTabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Library", theme.ListIcon(), LibraryTabUI(myWindow, authService, dm)),
		container.NewTabItemWithIcon("Downloads", theme.DownloadIcon(), DownloadsTabUI(dm)),
		container.NewTabItemWithIcon("File Ops", theme.DocumentIcon(), FileTabUI(myWindow)),
		container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), SettingsTabUI(myWindow)),
		container.NewTabItemWithIcon("About", theme.InfoIcon(), ShowAboutUI(version)),
	)
	mainTabs.SetTabLocation(container.TabLocationTop)

	myWindow.SetContent(mainTabs)
	myWindow.Resize(fyne.NewSize(960, 640))
	myWindow.ShowAndRun()
}

func FileTabUI(win fyne.Window) fyne.CanvasObject {
	head := widget.NewLabelWithStyle("File Operations", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	hashTab := HashUI(win)
	sizeTab := SizeUI(win)
	fileTabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Hash Files", theme.ContentAddIcon(), hashTab),
		container.NewTabItemWithIcon("Storage Size", theme.ViewFullScreenIcon(), sizeTab),
	)
	fileTabs.SetTabLocation(container.TabLocationTop)
	return container.NewBorder(
		head, nil, nil, nil,
		fileTabs,
	)
}
