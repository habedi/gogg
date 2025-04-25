// ui/ui.go
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog" // Added for dialog inside search button
	"fyne.io/fyne/v2/widget"
)

// Run function remains the same...
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

	// --- Search tab ---
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Enter game title or ID to search...")
	searchByIDCheck := widget.NewCheck("Search by ID", nil)
	initialResultsLabel := widget.NewLabel("Search results will appear here...") // Store initial label
	searchScroll := container.NewScroll(initialResultsLabel)                     // Use initial label here

	// Define the clear action here, so it can be passed to SearchCatalogueUI
	clearSearch := func() {
		searchEntry.SetText("")                    // Clear the search entry
		searchByIDCheck.SetChecked(false)          // Uncheck the ID box
		searchScroll.Content = initialResultsLabel // Reset scroll content to the initial label
		searchScroll.Refresh()                     // Refresh the scroll container
	}

	searchButton := widget.NewButton("Search", func() {
		query := searchEntry.Text
		searchByID := searchByIDCheck.Checked
		// Don't perform search if query is empty
		if query == "" {
			dialog.ShowInformation("Search", "Please enter a search query.", win)
			return
		}
		// *** Call SearchCatalogueUI and pass the clearSearch function ***
		results := SearchCatalogueUI(win, query, searchByID, clearSearch)
		searchScroll.Content = results // Update scroll content with results table or error message
		searchScroll.Refresh()
	})

	// *** REMOVED the Clear button from here ***
	// *** Grid is back to 3 columns ***
	searchBar := container.NewGridWithColumns(3, searchEntry, searchByIDCheck, searchButton)
	searchTab := container.NewBorder(searchBar, nil, nil, nil, searchScroll)

	// --- Refresh tab (remains the same) ---
	refreshButton := widget.NewButton("Refresh Catalogue", func() {
		RefreshCatalogueUI(win)
	})
	refreshTab := container.NewVBox(
		widget.NewLabel("Retrieve the latest data and refresh the catalogue:"),
		refreshButton,
	)

	// --- Export tab (remains the same) ---
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

	// --- Main Catalogue Tabs (remains the same) ---
	catalogueTabs := container.NewAppTabs(
		container.NewTabItem("Search", searchTab),
		container.NewTabItem("Refresh", refreshTab),
		container.NewTabItem("Game List", listTab),
		container.NewTabItem("Export", exportTab),
	)
	catalogueTabs.SetTabLocation(container.TabLocationTop)
	// Set Search tab as default
	catalogueTabs.SelectIndex(0)
	return catalogueTabs
}

// DownloadTabUI function remains the same...
func DownloadTabUI(win fyne.Window) fyne.CanvasObject {
	return DownloadUI(win)
}

// FileTabUI function remains the same...
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
