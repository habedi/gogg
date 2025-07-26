package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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

func LibraryTabUI(win fyne.Window, authService *auth.Service, dm *DownloadManager) fyne.CanvasObject {
	token, _ := db.GetTokenRecord()
	if token == nil {
		return container.NewCenter(container.NewVBox(
			widget.NewIcon(theme.WarningIcon()),
			widget.NewLabelWithStyle("Not logged in.", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabel("Please run 'gogg login' from your terminal to authenticate."),
		))
	}

	allGames, _ := db.GetCatalogue()
	gamesListBinding := binding.NewUntypedList()
	if len(allGames) > 0 {
		_ = gamesListBinding.Set(untypedSlice(allGames))
	}

	selectedGameBinding := binding.NewUntyped()

	var listPlaceholder fyne.CanvasObject
	var gameListWidget *widget.List

	// --- LEFT PANE (MASTER) ---
	var debounceTimer *time.Timer
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Type game title to search...")
	searchEntry.OnChanged = func(s string) {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(200*time.Millisecond, func() {
			runOnMain(func() {
				searchStr := strings.ToLower(s)
				if searchStr == "" {
					_ = gamesListBinding.Set(untypedSlice(allGames))
					return
				}
				filtered := make([]interface{}, 0)
				for _, game := range allGames {
					if strings.Contains(strings.ToLower(game.Title), searchStr) {
						filtered = append(filtered, game)
					}
				}
				_ = gamesListBinding.Set(filtered)
			})
		})
	}

	refreshUI := func() {
		allGames, _ = db.GetCatalogue()
		_ = gamesListBinding.Set(untypedSlice(allGames))
		if gameListWidget != nil {
			gameListWidget.Refresh()
		}
		if gamesListBinding.Length() > 0 && listPlaceholder != nil {
			listPlaceholder.Hide()
			if gameListWidget != nil {
				gameListWidget.Show()
			}
		}
	}

	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		searchEntry.SetText("")
		RefreshCatalogueAction(win, authService, refreshUI)
	})

	var exportBtn *widget.Button
	exportBtn = widget.NewButtonWithIcon("Export", theme.DocumentSaveIcon(), func() {
		popup := widget.NewPopUpMenu(fyne.NewMenu("",
			fyne.NewMenuItem("Export Game List as CSV", func() { ExportCatalogueAction(win, "csv") }),
			fyne.NewMenuItem("Export Full Catalogue as JSON", func() { ExportCatalogueAction(win, "json") }),
		), win.Canvas())
		popup.ShowAtPosition(win.Content().Position().Add(fyne.NewPos(exportBtn.Position().X, exportBtn.Position().Y+exportBtn.Size().Height)))
	})
	toolbar := container.NewHBox(refreshBtn, exportBtn)

	listContent := container.NewStack()
	if gamesListBinding.Length() == 0 {
		listPlaceholder = container.NewCenter(container.NewVBox(
			widget.NewIcon(theme.InfoIcon()),
			widget.NewLabel("Your library is empty or hasn't been synced."),
			widget.NewButton("Refresh Catalogue Now", func() {
				RefreshCatalogueAction(win, authService, refreshUI)
			}),
		))
		listContent.Add(listPlaceholder)
	} else {
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
		listContent.Add(gameListWidget)
	}

	leftPane := container.NewBorder(
		container.NewVBox(searchEntry, widget.NewSeparator()),
		toolbar, nil, nil,
		listContent,
	)

	// --- RIGHT PANE (DETAIL) ---
	detailTitle := widget.NewLabelWithStyle("Select a game from the list", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
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

	return container.NewHSplit(leftPane, rightPane)
}

func untypedSlice(games []db.Game) []interface{} {
	out := make([]interface{}, len(games))
	for i, g := range games {
		out[i] = g
	}
	return out
}

func createDetailsAccordion(win fyne.Window, authService *auth.Service, dm *DownloadManager, selectedGame binding.Untyped) *widget.Accordion {
	// Details View
	detailsLabel := widget.NewLabel("Game details will appear here.")
	detailsLabel.Wrapping = fyne.TextWrapWord

	// Download Form
	downloadForm := createDownloadForm(win, authService, dm, selectedGame)

	accordion := widget.NewAccordion(
		widget.NewAccordionItem("Download Options", downloadForm),
	)
	accordion.Open(0) // Open download options by default

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
	downloadPathEntry.SetText(prefs.StringWithFallback("downloadForm.path", ""))
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

	return container.NewVBox(form, checkboxes, layout.NewSpacer(), downloadBtn)
}
