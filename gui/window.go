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
	myWindow := myApp.NewWindow("Gogg GUI")

	mainTabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Catalogue", theme.ListIcon(), CatalogueTabUI(myWindow)),
		container.NewTabItemWithIcon("Download", theme.DownloadIcon(), DownloadTabUI(myWindow)),
		container.NewTabItemWithIcon("File", theme.DocumentIcon(), FileTabUI(myWindow)),
		container.NewTabItemWithIcon("About", theme.InfoIcon(), ShowAboutUI(version)),
	)
	mainTabs.SetTabLocation(container.TabLocationTop)

	myWindow.SetContent(mainTabs)
	myWindow.Resize(fyne.NewSize(1000, 600))
	myWindow.ShowAndRun()
}

// CatalogueTabUI builds the Catalogue tab with subtabs and icon buttons.
func CatalogueTabUI(win fyne.Window) fyne.CanvasObject {
	listTab := CatalogueListUI(win)

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Enter game title or ID to search...")
	searchByID := widget.NewCheck("Search by ID", nil)
	initialLabel := widget.NewLabel("Search results will appear here...")
	searchScroll := container.NewScroll(initialLabel)

	clearSearch := func() {
		searchEntry.SetText("")
		searchByID.SetChecked(false)
		searchScroll.Content = initialLabel
		searchScroll.Refresh()
	}

	searchBtn := widget.NewButtonWithIcon(
		"Search", theme.SearchIcon(), func() {
			q := searchEntry.Text
			if q == "" {
				dialog.ShowInformation("Search", "Please enter a search query.", win)
				return
			}
			results := SearchCatalogueUI(win, q, searchByID.Checked, clearSearch)
			searchScroll.Content = results
			searchScroll.Refresh()
		},
	)

	searchBar := container.NewGridWithColumns(3, searchEntry, searchByID, searchBtn)
	searchTab := container.NewBorder(searchBar, nil, nil, nil, searchScroll)

	refreshBtn := widget.NewButtonWithIcon(
		"Refresh Catalogue", theme.ViewRefreshIcon(), func() {
			RefreshCatalogueUI(win)
		},
	)
	refreshTab := container.NewVBox(
		widget.NewLabel("Retrieve the latest data and refresh the catalogue:"),
		refreshBtn,
	)

	exportJSON := widget.NewButtonWithIcon(
		"Export JSON", theme.ContentAddIcon(), func() {
			ExportCatalogueUI(win, "json")
		},
	)
	exportCSV := widget.NewButtonWithIcon(
		"Export CSV", theme.DocumentSaveIcon(), func() {
			ExportCatalogueUI(win, "csv")
		},
	)
	exportTab := container.NewVBox(
		widget.NewLabel("Export the full game catalogue or just the game list to a file:"),
		container.NewHBox(exportJSON, exportCSV),
	)

	catalogueTabs := container.NewAppTabs(
		container.NewTabItem("Search", searchTab),
		container.NewTabItem("Refresh", refreshTab),
		container.NewTabItem("Game List", listTab),
		container.NewTabItem("Export", exportTab),
	)
	catalogueTabs.SetTabLocation(container.TabLocationTop)
	catalogueTabs.SelectIndex(0)
	return catalogueTabs
}

// DownloadTabUI returns the Download tab with icon.
func DownloadTabUI(win fyne.Window) fyne.CanvasObject {
	return DownloadUI(win)
}

// FileTabUI returns the File tab with icon.
func FileTabUI(win fyne.Window) fyne.CanvasObject {
	hashTab := HashUI(win)
	sizeTab := SizeUI(win)
	fileTabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Hash", theme.ContentAddIcon(), hashTab),
		container.NewTabItemWithIcon("Size", theme.ViewFullScreenIcon(), sizeTab),
	)
	fileTabs.SetTabLocation(container.TabLocationTop)
	return fileTabs
}
