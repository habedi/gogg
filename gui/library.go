package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// libraryTab holds all the components of the library tab UI.
type libraryTab struct {
	content     fyne.CanvasObject
	searchEntry *widget.Entry
}

func LibraryTabUI(win fyne.Window, authService *auth.Service, dm *DownloadManager) *libraryTab {
	token, _ := db.GetTokenRecord()
	if token == nil {
		content := container.NewCenter(container.NewVBox(
			widget.NewIcon(theme.WarningIcon()),
			widget.NewLabelWithStyle("Not logged in.", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabel("Please run 'gogg login' from your terminal to authenticate."),
		))
		return &libraryTab{content: content, searchEntry: widget.NewEntry()} // Return dummy entry
	}

	allGames, _ := db.GetCatalogue()
	gamesListBinding := binding.NewUntypedList()
	selectedGameBinding := binding.NewUntyped()
	isSortAscending := true

	gameCountLabel := widget.NewLabel("")

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Type game title to search...")
	clearSearchBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		searchEntry.SetText("")
	})
	searchEntry.ActionItem = clearSearchBtn
	clearSearchBtn.Hide()

	var gameListWidget *widget.List
	updateDisplayedGames := func() {
		searchTerm := strings.ToLower(searchEntry.Text)
		displayGames := make([]db.Game, len(allGames))
		copy(displayGames, allGames)

		if isSortAscending {
			sort.Slice(displayGames, func(i, j int) bool {
				return strings.ToLower(displayGames[i].Title) < strings.ToLower(displayGames[j].Title)
			})
		} else {
			sort.Slice(displayGames, func(i, j int) bool {
				return strings.ToLower(displayGames[i].Title) > strings.ToLower(displayGames[j].Title)
			})
		}

		if searchTerm != "" {
			filtered := make([]db.Game, 0)
			for _, game := range displayGames {
				if strings.Contains(strings.ToLower(game.Title), searchTerm) {
					filtered = append(filtered, game)
				}
			}
			displayGames = filtered
		}

		_ = gamesListBinding.Set(untypedSlice(displayGames))
		gameCountLabel.SetText(fmt.Sprintf("%d games found", len(displayGames)))
		if searchTerm == "" {
			clearSearchBtn.Hide()
		} else {
			clearSearchBtn.Show()
		}
	}

	searchEntry.OnChanged = func(s string) { updateDisplayedGames() }

	listContent := container.NewStack()
	gameListWidget = widget.NewListWithData(gamesListBinding,
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
		gameRaw, _ := gamesListBinding.GetValue(id)
		_ = selectedGameBinding.Set(gameRaw)
	}
	gameListWidget.OnUnselected = func(id widget.ListItemID) {
		_ = selectedGameBinding.Set(nil)
	}

	var refreshBtn *widget.Button
	onFinishRefresh := func() {
		allGames, _ = db.GetCatalogue()
		updateDisplayedGames()
		refreshBtn.Enable()
		listContent.Objects = []fyne.CanvasObject{gameListWidget}
		listContent.Refresh()
	}

	if len(allGames) == 0 {
		placeholder := container.NewCenter(container.NewVBox(
			widget.NewIcon(theme.InfoIcon()),
			widget.NewLabel("Your library is empty or hasn't been synced."),
			widget.NewButton("Refresh Catalogue Now", func() {
				refreshBtn.Disable()
				RefreshCatalogueAction(win, authService, onFinishRefresh)
			}),
		))
		listContent.Add(placeholder)
	} else {
		listContent.Add(gameListWidget)
	}
	updateDisplayedGames()

	refreshBtn = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		searchEntry.SetText("")
		refreshBtn.Disable()
		RefreshCatalogueAction(win, authService, onFinishRefresh)
	})

	var exportBtn *widget.Button
	exportBtn = widget.NewButtonWithIcon("Export", theme.DocumentSaveIcon(), func() {
		popup := widget.NewPopUpMenu(fyne.NewMenu("",
			fyne.NewMenuItem("Export Game List as CSV", func() { ExportCatalogueAction(win, "csv") }),
			fyne.NewMenuItem("Export Full Catalogue as JSON", func() { ExportCatalogueAction(win, "json") }),
		), win.Canvas())
		popup.ShowAtPosition(win.Content().Position().Add(fyne.NewPos(exportBtn.Position().X, exportBtn.Position().Y+exportBtn.Size().Height)))
	})

	var sortBtn *widget.Button
	sortBtn = widget.NewButton("Sort A-Z", func() {
		isSortAscending = !isSortAscending
		if isSortAscending {
			sortBtn.SetText("Sort A-Z")
		} else {
			sortBtn.SetText("Sort Z-A")
		}
		updateDisplayedGames()
		gameListWidget.Refresh()
	})

	toolbar := container.NewHBox(refreshBtn, exportBtn, sortBtn, layout.NewSpacer(), gameCountLabel)
	leftTopContainer := container.NewVBox(searchEntry, widget.NewSeparator())
	leftPane := container.NewBorder(leftTopContainer, toolbar, nil, nil, listContent)

	detailTitle := NewCopyableLabel("Select a game from the list")
	detailTitle.Alignment = fyne.TextAlignCenter
	detailTitle.TextStyle = fyne.TextStyle{Bold: true}

	accordion := createDetailsAccordion(win, authService, dm, selectedGameBinding)
	rightPane := container.NewBorder(
		container.NewVBox(detailTitle, widget.NewSeparator()),
		nil, nil, nil,
		accordion,
	)
	accordion.Hide()

	selectedGameBinding.AddListener(binding.NewDataListener(func() {
		gameRaw, _ := selectedGameBinding.Get()
		if gameRaw == nil {
			accordion.Hide()
			detailTitle.SetText("Select a game from the list")
			return
		}
		game := gameRaw.(db.Game)
		detailTitle.SetText(game.Title)
		accordion.Show()
	}))

	return &libraryTab{
		content:     container.NewHSplit(leftPane, rightPane),
		searchEntry: searchEntry,
	}
}

func untypedSlice(games []db.Game) []interface{} {
	out := make([]interface{}, len(games))
	for i, g := range games {
		out[i] = g
	}
	return out
}

func createDetailsAccordion(win fyne.Window, authService *auth.Service, dm *DownloadManager, selectedGame binding.Untyped) *widget.Accordion {
	detailsLabel := widget.NewLabel("Game details will appear here.")
	detailsLabel.Wrapping = fyne.TextWrapWord

	downloadForm := createDownloadForm(win, authService, dm, selectedGame)

	accordion := widget.NewAccordion(
		widget.NewAccordionItem("Download Options", downloadForm),
	)
	accordion.Open(0)

	selectedGame.AddListener(binding.NewDataListener(func() {
		gameRaw, _ := selectedGame.Get()
		if gameRaw == nil {
			detailsLabel.SetText("Select a game to see its details.")
			return
		}
		game := gameRaw.(db.Game)

		var gameDetails map[string]interface{}
		if err := json.Unmarshal([]byte(game.Data), &gameDetails); err != nil {
			detailsLabel.SetText("Error parsing game details.")
			return
		}

		desc, ok := gameDetails["description"].(map[string]interface{})
		if ok {
			fullDesc, _ := desc["full"].(string)
			detailsLabel.SetText(fullDesc)
		} else {
			detailsLabel.SetText("No description available.")
		}
	}))

	return accordion
}

func createDownloadForm(win fyne.Window, authService *auth.Service, dm *DownloadManager, selectedGame binding.Untyped) fyne.CanvasObject {
	prefs := fyne.CurrentApp().Preferences()

	downloadPathEntry := widget.NewEntry()
	lastUsedPath := prefs.String("lastUsedDownloadPath")
	if lastUsedPath == "" {
		lastUsedPath = prefs.StringWithFallback("downloadForm.path", "")
	}
	downloadPathEntry.SetText(lastUsedPath)

	downloadPathEntry.OnChanged = func(s string) {
		prefs.SetString("downloadForm.path", s)
	}
	downloadPathEntry.SetPlaceHolder("Enter download path")

	browseBtn := widget.NewButton("Browse...", func() {
		folderDialog := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			downloadPathEntry.SetText(uri.Path())
		}, win)
		folderDialog.Resize(fyne.NewSize(800, 600))
		folderDialog.Show()
	})
	pathContainer := container.NewBorder(nil, nil, nil, browseBtn, downloadPathEntry)

	langCodes := make([]string, 0, len(client.GameLanguages))
	for code := range client.GameLanguages {
		langCodes = append(langCodes, code)
	}
	sort.Strings(langCodes)
	langSelect := widget.NewSelect(langCodes, func(s string) {
		prefs.SetString("downloadForm.language", s)
	})
	langSelect.SetSelected(prefs.StringWithFallback("downloadForm.language", "en"))

	platformSelect := widget.NewSelect([]string{"windows", "mac", "linux", "all"}, func(s string) {
		prefs.SetString("downloadForm.platform", s)
	})
	platformSelect.SetSelected(prefs.StringWithFallback("downloadForm.platform", "windows"))

	threadOptions := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	threadsSelect := widget.NewSelect(threadOptions, func(s string) {
		prefs.SetString("downloadForm.threads", s)
	})
	threadsSelect.SetSelected(prefs.StringWithFallback("downloadForm.threads", "5"))

	extrasCheck := widget.NewCheck("Include Extras", func(b bool) {
		prefs.SetBool("downloadForm.extras", b)
	})
	extrasCheck.SetChecked(prefs.BoolWithFallback("downloadForm.extras", true))

	dlcsCheck := widget.NewCheck("Include DLCs", func(b bool) {
		prefs.SetBool("downloadForm.dlcs", b)
	})
	dlcsCheck.SetChecked(prefs.BoolWithFallback("downloadForm.dlcs", true))

	resumeCheck := widget.NewCheck("Resume Downloads", func(b bool) {
		prefs.SetBool("downloadForm.resume", b)
	})
	resumeCheck.SetChecked(prefs.BoolWithFallback("downloadForm.resume", true))

	flattenCheck := widget.NewCheck("Flatten Directory", func(b bool) {
		prefs.SetBool("downloadForm.flatten", b)
	})
	flattenCheck.SetChecked(prefs.BoolWithFallback("downloadForm.flatten", true))

	skipPatchesCheck := widget.NewCheck("Skip Patches", func(b bool) {
		prefs.SetBool("downloadForm.skipPatches", b)
	})
	skipPatchesCheck.SetChecked(prefs.BoolWithFallback("downloadForm.skipPatches", true))

	gogdbBtn := widget.NewButtonWithIcon("View on gogdb.org", theme.SearchIcon(), nil)
	gogdbBtn.OnTapped = func() {
		gameRaw, _ := selectedGame.Get()
		if gameRaw == nil {
			return
		}
		game := gameRaw.(db.Game)
		gogdbURL := fmt.Sprintf("https://www.gogdb.org/product/%d", game.ID)
		if err := fyne.CurrentApp().OpenURL(parseURL(gogdbURL)); err != nil {
			dialog.ShowError(fmt.Errorf("failed to open gogdb URL: %w", err), win)
		}
	}

	downloadBtn := widget.NewButtonWithIcon("Download Game", theme.DownloadIcon(), nil)
	downloadBtn.Importance = widget.HighImportance
	downloadBtn.OnTapped = func() {
		if downloadPathEntry.Text == "" {
			showErrorDialog(win, "Download path cannot be empty.", nil)
			return
		}

		gameRaw, _ := selectedGame.Get()
		game := gameRaw.(db.Game)
		threads, _ := strconv.Atoi(threadsSelect.Selected)
		langFull := client.GameLanguages[langSelect.Selected]

		err := executeDownload(
			authService, dm, game,
			downloadPathEntry.Text, langFull, platformSelect.Selected,
			extrasCheck.Checked, dlcsCheck.Checked, resumeCheck.Checked,
			flattenCheck.Checked, skipPatchesCheck.Checked,
			threads,
		)

		if err != nil {
			if errors.Is(err, ErrDownloadInProgress) {
				dialog.ShowInformation("In Progress", "This game is already being downloaded.", win)
			} else {
				showErrorDialog(win, "Failed to start download", err)
			}
		} else {
			dialog.ShowInformation("Started", fmt.Sprintf("Download for '%s' has started.", game.Title), win)
		}
	}

	form := widget.NewForm(
		widget.NewFormItem("Download Path", pathContainer),
		widget.NewFormItem("Platform", platformSelect),
		widget.NewFormItem("Language", langSelect),
		widget.NewFormItem("Threads", threadsSelect),
	)

	checkboxes := container.New(layout.NewGridLayout(3), extrasCheck, dlcsCheck, skipPatchesCheck, resumeCheck, flattenCheck)

	return container.NewVBox(form, checkboxes, layout.NewSpacer(), gogdbBtn, downloadBtn)
}
