package gui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

func LibraryTabUI(win fyne.Window, authService *auth.Service, dm *DownloadManager) fyne.CanvasObject {
	allGames, _ := db.GetCatalogue()
	gamesList := binding.NewUntypedList()
	gamesList.Set(untypedSlice(allGames))

	selectedGame := binding.NewUntyped()

	// --- LEFT PANE (MASTER) ---
	gameListWidget := widget.NewListWithData(gamesList,
		func() fyne.CanvasObject {
			return widget.NewLabel("Game Title")
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			gameRaw, _ := item.(binding.Untyped).Get()
			game := gameRaw.(db.Game)
			obj.(*widget.Label).SetText(game.Title)
		},
	)

	gameListWidget.OnSelected = func(id widget.ListItemID) {
		gameRaw, _ := gamesList.GetValue(id)
		selectedGame.Set(gameRaw)
	}
	gameListWidget.OnUnselected = func(id widget.ListItemID) {
		selectedGame.Set(nil)
	}

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Search catalogue...")
	searchEntry.OnChanged = func(s string) {
		s = strings.ToLower(s)
		if s == "" {
			gamesList.Set(untypedSlice(allGames))
			return
		}
		filtered := make([]interface{}, 0)
		for _, game := range allGames {
			if strings.Contains(strings.ToLower(game.Title), s) {
				filtered = append(filtered, game)
			}
		}
		gamesList.Set(filtered)
	}

	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		searchEntry.SetText("")
		RefreshCatalogueAction(win, authService, func() {
			allGames, _ = db.GetCatalogue()
			gamesList.Set(untypedSlice(allGames))
		})
	})

	var exportBtn *widget.Button
	exportBtn = widget.NewButtonWithIcon("Export", theme.DocumentSaveIcon(), func() {
		popup := widget.NewPopUpMenu(fyne.NewMenu("",
			fyne.NewMenuItem("Export as CSV", func() { ExportCatalogueAction(win, "csv") }),
			fyne.NewMenuItem("Export as JSON", func() { ExportCatalogueAction(win, "json") }),
		), win.Canvas())
		popup.ShowAtPosition(win.Content().Position().Add(fyne.NewPos(exportBtn.Position().X, exportBtn.Position().Y+exportBtn.Size().Height)))
	})
	toolbar := container.NewHBox(refreshBtn, exportBtn)

	leftPane := container.NewBorder(
		container.NewVBox(searchEntry, widget.NewSeparator()),
		toolbar, nil, nil,
		gameListWidget,
	)

	// --- RIGHT PANE (DETAIL) ---
	detailTitle := widget.NewLabelWithStyle("Select a game from the list", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	downloadForm := createDownloadForm(win, authService, dm, selectedGame)
	rightPane := container.NewBorder(
		container.NewVBox(detailTitle, widget.NewSeparator()),
		nil, nil, nil,
		container.NewVScroll(downloadForm),
	)
	downloadForm.Hide()

	selectedGame.AddListener(binding.NewDataListener(func() {
		gameRaw, _ := selectedGame.Get()
		if gameRaw == nil {
			downloadForm.Hide()
			detailTitle.SetText("Select a game from the list")
			return
		}
		game := gameRaw.(db.Game)
		detailTitle.SetText(game.Title)
		downloadForm.Show()
	}))

	return container.NewHSplit(leftPane, rightPane)
}

func untypedSlice(games []db.Game) []interface{} {
	out := make([]interface{}, len(games))
	for i, g := range games {
		out[i] = g
	}
	return out
}

func createDownloadForm(win fyne.Window, authService *auth.Service, dm *DownloadManager, selectedGame binding.Untyped) fyne.CanvasObject {
	downloadPathEntry := widget.NewEntry()
	downloadPathEntry.SetPlaceHolder("Enter download path")

	langSelect := widget.NewSelect([]string{"en", "fr", "de", "es", "it", "ru", "pl", "pt-BR", "zh-Hans", "ja", "ko"}, nil)
	langSelect.SetSelected("en")

	platformSelect := widget.NewSelect([]string{"windows", "mac", "linux", "all"}, nil)
	platformSelect.SetSelected("windows")

	threadsEntry := widget.NewEntry()
	threadsEntry.Validator = validation.NewRegexp(`^[1-9]$|^1[0-9]$|^20$`, "Must be 1-20")
	threadsEntry.SetText("5")

	extrasCheck := widget.NewCheck("Include Extras", nil)
	extrasCheck.SetChecked(true)
	dlcsCheck := widget.NewCheck("Include DLCs", nil)
	dlcsCheck.SetChecked(true)
	resumeCheck := widget.NewCheck("Resume Downloads", nil)
	resumeCheck.SetChecked(true)
	flattenCheck := widget.NewCheck("Flatten Directory", nil)
	flattenCheck.SetChecked(false)

	downloadBtn := widget.NewButtonWithIcon("Download Game", theme.DownloadIcon(), nil)
	downloadBtn.Importance = widget.HighImportance
	downloadBtn.OnTapped = func() {
		if threadsEntry.Validate() != nil {
			showErrorDialog(win, "Invalid number of threads. Must be between 1 and 20.", nil)
			return
		}
		if downloadPathEntry.Text == "" {
			showErrorDialog(win, "Download path cannot be empty.", nil)
			return
		}

		gameRaw, _ := selectedGame.Get()
		game := gameRaw.(db.Game)
		threads, _ := strconv.Atoi(threadsEntry.Text)
		lang := langSelect.Selected
		langFull, _ := client.GameLanguages[lang]

		go executeDownload(
			authService, dm, game,
			downloadPathEntry.Text, langFull, platformSelect.Selected,
			extrasCheck.Checked, dlcsCheck.Checked, resumeCheck.Checked,
			flattenCheck.Checked, false, // skipPatches flag
			threads,
		)
		dialog.ShowInformation("Started", fmt.Sprintf("Download for '%s' has started.\nCheck the Downloads tab for progress.", game.Title), win)
	}

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Download Path", Widget: downloadPathEntry},
			{Text: "Platform", Widget: platformSelect},
			{Text: "Language", Widget: langSelect},
			{Text: "Threads", Widget: threadsEntry},
		},
	}

	return container.NewVBox(
		form,
		container.NewHBox(extrasCheck, dlcsCheck),
		container.NewHBox(resumeCheck, flattenCheck),
		layout.NewSpacer(),
		downloadBtn,
	)
}
