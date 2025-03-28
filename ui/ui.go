// ui/ui.go
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Run starts the Gogg GUI with Catalogue, Download, and File tabs.
func Run() {
	myApp := app.NewWithID("com.habedi.gogg")
	myWindow := myApp.NewWindow("Gogg GUI")

	mainTabs := container.NewAppTabs(
		container.NewTabItem("Catalogue", CatalogueTabUI(myWindow)),
		container.NewTabItem("Download", DownloadTabUI(myWindow)),
		container.NewTabItem("File", FileTabUI(myWindow)),
	)
	mainTabs.SetTabLocation(container.TabLocationTop)

	myWindow.SetContent(mainTabs)
	myWindow.Resize(fyne.NewSize(900, 600))
	myWindow.ShowAndRun()
}

// CatalogueTabUI returns the catalogue UI content as a container with four sub-tabs.
func CatalogueTabUI(win fyne.Window) fyne.CanvasObject {
	// Game List tab.
	listTab := CatalogueListUI(win)

	// Search tab.
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Enter game title or ID to search...")
	searchByIDCheck := widget.NewCheck("Search by ID", nil)
	searchScroll := container.NewScroll(widget.NewLabel("Search results will appear here..."))
	searchButton := widget.NewButton("Search", func() {
		query := searchEntry.Text
		searchByID := searchByIDCheck.Checked
		results := SearchCatalogueUI(win, query, searchByID)
		searchScroll.Content = results
		searchScroll.Refresh()
	})
	searchBar := container.NewGridWithColumns(3, searchEntry, searchByIDCheck, searchButton)
	searchTab := container.NewBorder(searchBar, nil, nil, nil, searchScroll)

	// Refresh tab.
	refreshButton := widget.NewButton("Refresh Catalogue", func() {
		RefreshCatalogueUI(win)
	})
	refreshTab := container.NewVBox(
		widget.NewLabel("Retrieve the latest data and refresh the catalogue:"),
		refreshButton,
	)

	// Export tab.
	exportJSONButton := widget.NewButton("Export full catalogue", func() {
		ExportCatalogueUI(win, "json")
	})
	exportCSVButton := widget.NewButton("Export game list", func() {
		ExportCatalogueUI(win, "csv")
	})
	exportTab := container.NewVBox(
		widget.NewLabel("Export the full game catalogue or just the game list to a file:"),
		container.NewHBox(exportJSONButton, exportCSVButton),
	)

	catalogueTabs := container.NewAppTabs(
		container.NewTabItem("Search", searchTab),
		container.NewTabItem("Refresh", refreshTab),
		container.NewTabItem("Game List", listTab),
		container.NewTabItem("Export", exportTab),
	)
	catalogueTabs.SetTabLocation(container.TabLocationTop)
	return catalogueTabs
}

// DownloadTabUI returns the download UI content.
func DownloadTabUI(win fyne.Window) fyne.CanvasObject {
	// DownloadUI is defined in ui/download.go.
	return DownloadUI(win)
}

// FileTabUI returns the file operations UI content with sub-tabs for Hash and Size.
func FileTabUI(win fyne.Window) fyne.CanvasObject {
	hashTab := HashUI(win)
	sizeTab := SizeUI(win)
	fileTabs := container.NewAppTabs(
		container.NewTabItem("Hash", hashTab),
		container.NewTabItem("Size", sizeTab),
	)
	fileTabs.SetTabLocation(container.TabLocationTop)
	return fileTabs
}
