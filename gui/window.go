package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Run initializes and runs the main GUI application.
func Run(version string) {
	myApp := app.NewWithID("com.habedi.gogg")
	myWindow := myApp.NewWindow("GOGG GUI")

	mainTabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Game Catalogue", theme.ListIcon(), CatalogueTabUI(myWindow)),
		container.NewTabItemWithIcon("Download Game", theme.DownloadIcon(), DownloadTabUI(myWindow)),
		container.NewTabItemWithIcon("File Ops", theme.DocumentIcon(), FileTabUI(myWindow)),
		container.NewTabItemWithIcon("About", theme.InfoIcon(), ShowAboutUI(version)),
	)
	mainTabs.SetTabLocation(container.TabLocationTop)

	myWindow.SetContent(mainTabs)
	myWindow.Resize(fyne.NewSize(800, 600))
	myWindow.ShowAndRun()
}

// CatalogueTabUI builds the Catalogue tab with subtabs and icon buttons.
func CatalogueTabUI(win fyne.Window) fyne.CanvasObject {
	listTab := CatalogueListUI(win)

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Enter a search term or game ID to search")
	searchByID := widget.NewCheck("Search by ID", nil)
	initialLabel := widget.NewLabel("Search results will appear here")
	searchScroll := container.NewScroll(initialLabel)

	clearSearch := func() {
		searchEntry.SetText("")
		searchByID.SetChecked(false)
		searchScroll.Content = initialLabel
		searchScroll.Refresh()
	}

	searchBtn := widget.NewButtonWithIcon("Search Catalogue", theme.SearchIcon(), func() {
		q := searchEntry.Text
		if q == "" {
			dialog.ShowInformation("Search", "Enter a valid search term or game ID", win)
			return
		}
		results := SearchCatalogueUI(win, q, searchByID.Checked, clearSearch)
		searchScroll.Content = results
		searchScroll.Refresh()
	})
	searchBtn.Importance = widget.HighImportance

	searchBar := container.NewBorder(nil, nil, nil,
		container.NewHBox(searchByID, searchBtn),
		searchEntry,
	)

	searchTab := container.NewBorder(searchBar, nil, nil, nil, searchScroll)

	refreshBtn := widget.NewButtonWithIcon("Refresh Catalogue", theme.ViewRefreshIcon(), func() {
		RefreshCatalogueUI(win)
	})
	refreshBtn.Importance = widget.MediumImportance

	refreshTab := container.NewVBox(
		widget.NewLabelWithStyle("Rebuild the game catalogue with the latest data from GOG", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		refreshBtn,
	)

	exportJSON := widget.NewButtonWithIcon("Export Full Catalogue (JSON)", theme.DocumentSaveIcon(), func() {
		ExportCatalogueUI(win, "json")
	})
	exportJSON.Importance = widget.MediumImportance
	exportCSV := widget.NewButtonWithIcon("Export Game List (CSV)", theme.DocumentSaveIcon(), func() {
		ExportCatalogueUI(win, "csv")
	})
	exportCSV.Importance = widget.MediumImportance
	exportTab := container.NewVBox(
		widget.NewLabelWithStyle("Export the game catalogue data", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(exportJSON, exportCSV),
	)

	catalogueTabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Search", theme.SearchIcon(), searchTab),
		container.NewTabItemWithIcon("Refresh", theme.ViewRefreshIcon(), refreshTab),
		container.NewTabItemWithIcon("Game List", theme.ListIcon(), listTab),
		container.NewTabItemWithIcon("Export", theme.DocumentSaveIcon(), exportTab),
	)
	catalogueTabs.SetTabLocation(container.TabLocationTop)
	catalogueTabs.SelectIndex(0)
	return catalogueTabs
}

// DownloadTabUI returns the Download tab with a header and expanding content.
func DownloadTabUI(win fyne.Window) fyne.CanvasObject {
	head := widget.NewLabelWithStyle("Download Game Files", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	down := DownloadUI(win)
	return container.NewBorder(
		container.NewVBox(head, widget.NewSeparator()),
		nil, nil, nil,
		down,
	)
}

// FileTabUI returns the File tab with header and expanding tabs.
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
