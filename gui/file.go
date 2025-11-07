package gui

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/hasher"
	"github.com/habedi/gogg/pkg/operations"
	"github.com/rs/zerolog/log"
)

type hashResult struct {
	File string
	Hash string
}

type sizeResult struct {
	Key   string
	Value string
}

// hashRow is a custom widget for a single row in our hash results list.
type hashRow struct {
	widget.BaseWidget
	file, hash *CopyableLabel
	layout     fyne.Layout
}

// newHashRow creates a new row widget.
func newHashRow() *hashRow {
	row := &hashRow{
		file: NewCopyableLabel(""),
		hash: NewCopyableLabel(""),
	}
	row.layout = newColumnLayout()
	row.ExtendBaseWidget(row)
	return row
}

// SetTexts sets the text for the file and hash labels.
func (r *hashRow) SetTexts(file, hash string) {
	r.file.SetText(file)
	r.hash.SetText(hash)
}

// SetHeaderStyle applies bold styling for the header row.
func (r *hashRow) SetHeaderStyle() {
	r.file.TextStyle.Bold = true
	r.hash.TextStyle.Bold = true
}

func (r *hashRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.New(r.layout, r.file, r.hash))
}

// columnLayout defines a simple two-column layout with a fixed right column.
type columnLayout struct{}

const hashColWidth float32 = 530

func newColumnLayout() fyne.Layout {
	return &columnLayout{}
}

func (c *columnLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) != 2 {
		return
	}
	// Right column (hash)
	hashSize := fyne.NewSize(hashColWidth, objects[1].MinSize().Height)
	objects[1].Resize(hashSize)
	objects[1].Move(fyne.NewPos(size.Width-hashColWidth, 0))

	// Left column (file path)
	filePathSize := fyne.NewSize(size.Width-hashColWidth-theme.Padding(), objects[0].MinSize().Height)
	objects[0].Resize(filePathSize)
	objects[0].Move(fyne.NewPos(0, 0))
}

func (c *columnLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) != 2 {
		return fyne.Size{}
	}
	minWidth := objects[0].MinSize().Width + objects[1].MinSize().Width + theme.Padding()
	minHeight := float32(0)
	if h := objects[0].MinSize().Height; h > minHeight {
		minHeight = h
	}
	if h := objects[1].MinSize().Height; h > minHeight {
		minHeight = h
	}
	return fyne.NewSize(minWidth, minHeight)
}

func HashUI(win fyne.Window) fyne.CanvasObject {
	prefs := fyne.CurrentApp().Preferences()

	header := newHashRow()
	header.SetHeaderStyle()

	dirEntry := widget.NewEntry()
	dirEntry.SetText(prefs.StringWithFallback("hashUI.path", ""))
	dirEntry.OnChanged = func(s string) {
		prefs.SetString("hashUI.path", s)
	}
	dirEntry.SetPlaceHolder("Path to scan")

	browseBtn := widget.NewButton("Browse...", func() {
		folderDialog := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			dirEntry.SetText(uri.Path())
		}, win)
		folderDialog.Resize(fyne.NewSize(800, 600))
		folderDialog.Show()
	})
	pathContainer := container.NewBorder(nil, nil, nil, browseBtn, dirEntry)

	guiHashAlgos := make([]string, 0)
	for _, algo := range hasher.HashAlgorithms {
		if algo != "sha512" {
			guiHashAlgos = append(guiHashAlgos, algo)
		}
	}

	algoSelect := widget.NewSelect(guiHashAlgos, func(s string) {
		prefs.SetString("hashUI.algo", s)
		header.SetTexts("File Path", fmt.Sprintf("Hash (%s)", s))
	})
	initialAlgo := prefs.StringWithFallback("hashUI.algo", "md5")
	algoSelect.SetSelected(initialAlgo)
	header.SetTexts("File Path", fmt.Sprintf("Hash (%s)", initialAlgo))

	threadsSelect := widget.NewSelect([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, func(s string) {
		prefs.SetString("hashUI.threads", s)
	})
	threadsSelect.SetSelected(prefs.StringWithFallback("hashUI.threads", "4"))

	recursiveCheck := widget.NewCheck("Recursive", func(b bool) {
		prefs.SetBool("hashUI.recursive", b)
	})
	recursiveCheck.SetChecked(prefs.BoolWithFallback("hashUI.recursive", true))

	form := widget.NewForm(
		widget.NewFormItem("Directory", pathContainer),
		widget.NewFormItem("Algorithm", algoSelect),
		widget.NewFormItem("Threads", threadsSelect),
	)

	generateBtn := widget.NewButton("Generate File Hashes", nil)
	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	topContent := container.NewVBox(form, recursiveCheck, generateBtn, progressBar)

	resultsData := binding.NewUntypedList()

	resultsList := widget.NewListWithData(
		resultsData,
		func() fyne.CanvasObject {
			return newHashRow()
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			res := item.(binding.Untyped)
			val, _ := res.Get()
			row := obj.(*hashRow)
			row.SetTexts(val.(hashResult).File, val.(hashResult).Hash)
		},
	)

	generateBtn.OnTapped = func() {
		dir := dirEntry.Text
		if dir == "" {
			dialog.ShowError(errors.New("please select a directory"), win)
			return
		}
		if _, statErr := os.Stat(dir); statErr != nil {
			dialog.ShowError(fmt.Errorf("directory does not exist: %w", statErr), win)
			return
		}

		_ = resultsData.Set(make([]interface{}, 0))
		progressBar.SetValue(0)
		progressBar.Show()
		generateBtn.Disable()

		go func() {
			defer runOnMain(func() {
				generateBtn.Enable()
				progressBar.Hide()
			})
			numThreads, _ := strconv.Atoi(threadsSelect.Selected)
			generateHashFilesUI(dir, algoSelect.Selected, recursiveCheck.Checked, numThreads, resultsData, progressBar)
		}()
	}

	clearBtn := widget.NewButtonWithIcon("Clear Results", theme.DeleteIcon(), func() {
		_ = resultsData.Set(make([]interface{}, 0))
	})

	copyBtn := widget.NewButtonWithIcon("Copy All Results", theme.ContentCopyIcon(), func() {
		items, _ := resultsData.Get()
		if len(items) == 0 {
			fyne.CurrentApp().SendNotification(fyne.NewNotification("Gogg", "Nothing to copy."))
			return
		}

		var sb strings.Builder
		writer := csv.NewWriter(&sb)
		_ = writer.Write([]string{"File", "Hash"}) // Header

		for _, item := range items {
			res := item.(hashResult)
			_ = writer.Write([]string{res.File, res.Hash})
		}
		writer.Flush()

		fyne.CurrentApp().Clipboard().SetContent(sb.String())
		fyne.CurrentApp().SendNotification(fyne.NewNotification("Gogg", "Hash results copied to clipboard."))
	})

	bottomBar := container.NewHBox(layout.NewSpacer(), clearBtn, copyBtn)

	listHeader := container.NewVBox(header, widget.NewSeparator())
	listContainer := container.NewBorder(listHeader, nil, nil, nil, resultsList)

	return container.NewBorder(topContent, bottomBar, nil, nil, listContainer)
}

func generateHashFilesUI(dir, algo string, recursive bool, numThreads int, results binding.UntypedList, progress *widget.ProgressBar) {
	filesToProcess, err := operations.FindFilesToHash(dir, recursive, operations.DefaultHashExclusions)
	if err != nil {
		log.Error().Err(err).Msg("GUI: Failed to find files to hash")
		return
	}

	totalFiles := len(filesToProcess)
	if totalFiles == 0 {
		return
	}
	runOnMain(func() {
		progress.Max = float64(totalFiles)
	})

	var processedCount atomic.Int64
	resultsChan := operations.GenerateHashes(context.Background(), filesToProcess, algo, numThreads)

	for res := range resultsChan {
		if res.Err == nil {
			relativePath, relErr := filepath.Rel(dir, res.File)
			if relErr != nil {
				relativePath = filepath.Base(res.File)
			}
			runOnMain(func() {
				_ = results.Append(hashResult{File: relativePath, Hash: res.Hash})
			})
		} else {
			log.Error().Err(res.Err).Str("file", res.File).Msg("GUI: Error hashing file")
		}

		newCount := processedCount.Add(1)
		runOnMain(func() {
			progress.SetValue(float64(newCount))
		})
	}
}

func SizeUI(win fyne.Window) fyne.CanvasObject {
	prefs := fyne.CurrentApp().Preferences()

	var gameMap map[string]int
	var allGameTitles []string
	var filteredGameTitles []string
	selectedGames := make(map[string]bool)

	// Multi-select list for games
	gamesList := widget.NewList(
		func() int {
			return len(filteredGameTitles)
		},
		func() fyne.CanvasObject {
			check := widget.NewCheck("", nil)
			return container.NewHBox(check, widget.NewLabel("Game Title"))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(filteredGameTitles) {
				return
			}
			title := filteredGameTitles[id]
			hbox := obj.(*fyne.Container)
			check := hbox.Objects[0].(*widget.Check)
			label := hbox.Objects[1].(*widget.Label)

			label.SetText(title)
			check.SetChecked(selectedGames[title])
			check.OnChanged = func(checked bool) {
				selectedGames[title] = checked
			}
		},
	)

	refreshGameList := func() {
		games, err := db.GetCatalogue()
		if err != nil {
			log.Error().Err(err).Msg("Failed to reload catalogue for SizeUI")
			return
		}

		gameMap = make(map[string]int)
		allGameTitles = make([]string, len(games))
		for i, game := range games {
			gameMap[game.Title] = game.ID
			allGameTitles[i] = game.Title
		}
		sort.Strings(allGameTitles)
		filteredGameTitles = allGameTitles
		gamesList.Refresh()
	}

	listener := binding.NewDataListener(func() {
		runOnMain(refreshGameList)
	})
	catalogueUpdated.AddListener(listener)

	refreshGameList()

	filterEntry := widget.NewEntry()
	filterEntry.SetPlaceHolder("Type to filter games...")
	clearFilterBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		filterEntry.SetText("")
	})
	filterEntry.ActionItem = clearFilterBtn
	clearFilterBtn.Hide()

	gamesCountLabel := widget.NewLabel(fmt.Sprintf("Showing %d games", len(allGameTitles)))

	filterEntry.OnChanged = func(s string) {
		s = strings.ToLower(s)

		if s == "" {
			filteredGameTitles = allGameTitles
			clearFilterBtn.Hide()
		} else {
			filtered := make([]string, 0)
			for _, title := range allGameTitles {
				if strings.Contains(strings.ToLower(title), s) {
					filtered = append(filtered, title)
				}
			}
			filteredGameTitles = filtered
			clearFilterBtn.Show()
		}
		gamesCountLabel.SetText(fmt.Sprintf("Showing %d games", len(filteredGameTitles)))
		gamesList.Refresh()
	}

	selectAllBtn := widget.NewButton("Select All Shown", func() {
		for _, title := range filteredGameTitles {
			selectedGames[title] = true
		}
		gamesList.Refresh()
	})

	deselectAllBtn := widget.NewButton("Deselect All", func() {
		selectedGames = make(map[string]bool)
		gamesList.Refresh()
	})

	selectionControls := container.NewHBox(selectAllBtn, deselectAllBtn, layout.NewSpacer(), gamesCountLabel)

	langCodes := make([]string, 0, len(client.GameLanguages))
	for code := range client.GameLanguages {
		langCodes = append(langCodes, code)
	}
	sort.Strings(langCodes)
	langSelect := widget.NewSelect(langCodes, func(s string) {
		prefs.SetString("sizeUI.language", s)
	})
	langSelect.SetSelected(prefs.StringWithFallback("sizeUI.language", "en"))

	platformSelect := widget.NewSelect([]string{"all", "windows", "mac", "linux"}, func(s string) {
		prefs.SetString("sizeUI.platform", s)
	})
	platformSelect.SetSelected(prefs.StringWithFallback("sizeUI.platform", "windows"))

	unitSelect := widget.NewSelect([]string{"gb", "mb"}, func(s string) {
		prefs.SetString("sizeUI.unit", s)
	})
	unitSelect.SetSelected(prefs.StringWithFallback("sizeUI.unit", "gb"))

	extrasCheck := widget.NewCheck("Include Extras", func(b bool) {
		prefs.SetBool("sizeUI.extras", b)
	})
	extrasCheck.SetChecked(prefs.BoolWithFallback("sizeUI.extras", true))

	dlcsCheck := widget.NewCheck("Include DLCs", func(b bool) {
		prefs.SetBool("sizeUI.dlcs", b)
	})
	dlcsCheck.SetChecked(prefs.BoolWithFallback("sizeUI.dlcs", true))

	form := widget.NewForm(
		widget.NewFormItem("Language", langSelect),
		widget.NewFormItem("Platform", platformSelect),
		widget.NewFormItem("Unit", unitSelect),
	)

	progressBar := widget.NewProgressBar()
	progressBar.Hide()
	statusLabel := widget.NewLabel("")

	estimateSelectedBtn := widget.NewButton("Estimate Selected Games", nil)
	estimateSelectedBtn.Importance = widget.HighImportance

	estimateAllFilteredBtn := widget.NewButton("Estimate All Filtered Games", nil)
	estimateAllFilteredBtn.Importance = widget.MediumImportance

	// Add help button
	helpBtn := widget.NewButtonWithIcon("Help", theme.HelpIcon(), func() {
		helpContent := widget.NewLabel(
			"How to use Storage Size Estimation:\n\n" +
				"1. Filter games: Type in the search box to filter the game list\n\n" +
				"2. Select games:\n" +
				"   • Click checkboxes to select individual games\n" +
				"   • Use 'Select All Shown' to select all filtered games\n" +
				"   • Use 'Deselect All' to clear all selections\n\n" +
				"3. Configure settings:\n" +
				"   • Choose language, platform, and display unit\n" +
				"   • Toggle extras and DLCs inclusion\n\n" +
				"4. Estimate:\n" +
				"   • Click 'Estimate Selected' for chosen games\n" +
				"   • Click 'Estimate All Filtered' for all visible games\n\n" +
				"5. Results:\n" +
				"   • View individual game sizes in the table below\n" +
				"   • See total estimated size in the summary\n" +
				"   • Copy results to clipboard as CSV",
		)
		helpContent.Wrapping = fyne.TextWrapWord

		helpDialog := dialog.NewCustom("Storage Size Estimation Help", "Close",
			container.NewVScroll(helpContent), win)
		helpDialog.Resize(fyne.NewSize(600, 500))
		helpDialog.Show()
	})

	buttonRow := container.NewHBox(
		estimateSelectedBtn,
		estimateAllFilteredBtn,
		layout.NewSpacer(),
		helpBtn,
	)

	// Left panel with scrollable game list
	leftHeaderLabel := widget.NewLabel("Filter and Select Games:")
	leftHeaderLabel.TextStyle.Bold = true

	leftPanelTop := container.NewVBox(
		leftHeaderLabel,
		widget.NewSeparator(),
		filterEntry,
		selectionControls,
		widget.NewSeparator(),
	)

	// Create a card-like container for the games list with subtle background
	gamesListCard := widget.NewCard("", "", container.NewScroll(gamesList))

	leftPanel := container.NewBorder(
		leftPanelTop,
		nil,
		nil,
		nil,
		gamesListCard,
	)

	// Right panel with properly aligned settings
	rightHeaderLabel := widget.NewLabel("Estimation Settings:")
	rightHeaderLabel.TextStyle.Bold = true

	// Wrap form in a card for visual grouping
	formCard := widget.NewCard("", "",
		container.NewVBox(
			form,
			container.NewHBox(extrasCheck, dlcsCheck),
		),
	)

	rightPanelTop := container.NewVBox(
		rightHeaderLabel,
		widget.NewSeparator(),
		formCard,
		buttonRow,
		progressBar,
		statusLabel,
	)

	rightPanel := container.NewBorder(
		rightPanelTop,
		nil,
		nil,
		nil,
		layout.NewSpacer(),
	)

	topContent := container.NewHSplit(leftPanel, rightPanel)
	topContent.SetOffset(0.5)

	resultsData := binding.NewUntypedList()

	// Create a table with better sizing
	resultsTable := widget.NewTable(
		func() (int, int) { return resultsData.Length(), 2 },
		func() fyne.CanvasObject {
			label := NewCopyableLabel("Template")
			label.Wrapping = fyne.TextWrapWord
			return label
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			item, err := resultsData.GetValue(id.Row)
			if err != nil {
				return
			}
			res := item.(sizeResult)
			label := cell.(*CopyableLabel)
			if id.Col == 0 {
				label.SetText(res.Key)
				label.TextStyle.Bold = true
			} else {
				label.SetText(res.Value)
				label.TextStyle.Bold = false
			}
			label.Refresh()
		},
	)
	resultsTable.SetColumnWidth(0, 500)
	resultsTable.SetColumnWidth(1, 200)

	estimateSelectedBtn.OnTapped = func() {
		selectedTitles := make([]string, 0)
		for title, selected := range selectedGames {
			if selected {
				selectedTitles = append(selectedTitles, title)
			}
		}

		if len(selectedTitles) == 0 {
			dialog.ShowError(errors.New("please select at least one game"), win)
			return
		}

		estimateSelectedBtn.Disable()
		estimateAllFilteredBtn.Disable()
		progressBar.Show()

		go func() {
			defer runOnMain(func() {
				estimateSelectedBtn.Enable()
				estimateAllFilteredBtn.Enable()
				progressBar.Hide()
				statusLabel.SetText("Estimation complete")
			})

			estimateMultipleGamesUI(
				selectedTitles,
				gameMap,
				langSelect.Selected,
				platformSelect.Selected,
				extrasCheck.Checked,
				dlcsCheck.Checked,
				unitSelect.Selected,
				resultsData,
				progressBar,
				statusLabel,
			)
		}()
	}

	estimateAllFilteredBtn.OnTapped = func() {
		if len(filteredGameTitles) == 0 {
			dialog.ShowError(errors.New("no games match the filter"), win)
			return
		}

		msg := fmt.Sprintf("Estimate storage size for all %d filtered games?", len(filteredGameTitles))
		dialog.ShowConfirm("Confirm Estimation", msg, func(confirmed bool) {
			if !confirmed {
				return
			}

			estimateSelectedBtn.Disable()
			estimateAllFilteredBtn.Disable()
			progressBar.Show()

			go func() {
				defer runOnMain(func() {
					estimateSelectedBtn.Enable()
					estimateAllFilteredBtn.Enable()
					progressBar.Hide()
					statusLabel.SetText("Estimation complete")
				})

				estimateMultipleGamesUI(
					filteredGameTitles,
					gameMap,
					langSelect.Selected,
					platformSelect.Selected,
					extrasCheck.Checked,
					dlcsCheck.Checked,
					unitSelect.Selected,
					resultsData,
					progressBar,
					statusLabel,
				)
			}()
		}, win)
	}

	clearBtn := widget.NewButtonWithIcon("Clear Results", theme.DeleteIcon(), func() {
		_ = resultsData.Set(make([]interface{}, 0))
		statusLabel.SetText("")
	})
	clearBtn.Importance = widget.LowImportance

	copyBtn := widget.NewButtonWithIcon("Copy All (CSV)", theme.ContentCopyIcon(), func() {
		items, _ := resultsData.Get()
		if len(items) == 0 {
			fyne.CurrentApp().SendNotification(fyne.NewNotification("Gogg", "Nothing to copy."))
			return
		}

		var sb strings.Builder
		writer := csv.NewWriter(&sb)
		_ = writer.Write([]string{"Game/Parameter", "Value"})

		for _, item := range items {
			res := item.(sizeResult)
			_ = writer.Write([]string{res.Key, res.Value})
		}
		writer.Flush()

		fyne.CurrentApp().Clipboard().SetContent(sb.String())
		fyne.CurrentApp().SendNotification(fyne.NewNotification("Gogg", "Size estimation results copied to clipboard."))
	})
	copyBtn.Importance = widget.MediumImportance

	bottomBar := container.NewHBox(layout.NewSpacer(), clearBtn, copyBtn)

	// Wrap results in a card with a header for better visual organization
	resultsHeader := widget.NewLabel("Estimation Results:")
	resultsHeader.TextStyle.Bold = true

	resultsTableScroll := container.NewScroll(resultsTable)
	resultsCard := widget.NewCard("", "", resultsTableScroll)

	resultsSection := container.NewBorder(
		container.NewVBox(resultsHeader, widget.NewSeparator()),
		nil,
		nil,
		nil,
		resultsCard,
	)

	return container.NewBorder(topContent, bottomBar, nil, nil, resultsSection)
}

func estimateMultipleGamesUI(
	gameTitles []string,
	gameMap map[string]int,
	languageCode, platformName string,
	extrasFlag, dlcFlag bool,
	sizeUnit string,
	results binding.UntypedList,
	progress *widget.ProgressBar,
	statusLabel *widget.Label,
) {
	_ = results.Set(make([]interface{}, 0))

	totalGames := len(gameTitles)
	runOnMain(func() {
		progress.Max = float64(totalGames)
		progress.SetValue(0)
		statusLabel.SetText(fmt.Sprintf("Estimating 0/%d games...", totalGames))
	})

	params := operations.EstimationParams{
		LanguageCode:  languageCode,
		PlatformName:  platformName,
		IncludeExtras: extrasFlag,
		IncludeDLCs:   dlcFlag,
	}

	var totalBytes int64
	successCount := 0
	errorCount := 0

	boolToStr := func(b bool) string {
		if b {
			return "Yes"
		}
		return "No"
	}

	// Add header information
	langFullName := client.GameLanguages[languageCode]
	headerRows := []interface{}{
		sizeResult{"=== Estimation Settings ===", ""},
		sizeResult{"Platform", platformName},
		sizeResult{"Language", langFullName},
		sizeResult{"Extras Included", boolToStr(extrasFlag)},
		sizeResult{"DLCs Included", boolToStr(dlcFlag)},
		sizeResult{"Total Games", fmt.Sprintf("%d", totalGames)},
		sizeResult{"", ""},
		sizeResult{"=== Individual Games ===", ""},
	}
	runOnMain(func() {
		_ = results.Set(headerRows)
	})

	for i, title := range gameTitles {
		gameID, exists := gameMap[title]
		if !exists {
			errorCount++
			runOnMain(func() {
				_ = results.Append(sizeResult{title, "Error: Game not found"})
			})
			continue
		}

		totalSizeBytes, _, err := operations.EstimateGameSize(gameID, params)
		if err != nil {
			errorCount++
			runOnMain(func() {
				_ = results.Append(sizeResult{title, fmt.Sprintf("Error: %v", err)})
			})
		} else {
			totalBytes += totalSizeBytes
			successCount++

			var sizeStr string
			if sizeUnit == "gb" {
				sizeInGB := float64(totalSizeBytes) / (1024 * 1024 * 1024)
				sizeStr = fmt.Sprintf("%.2f GB", sizeInGB)
			} else {
				sizeInMB := float64(totalSizeBytes) / (1024 * 1024)
				sizeStr = fmt.Sprintf("%.2f MB", sizeInMB)
			}

			runOnMain(func() {
				_ = results.Append(sizeResult{title, sizeStr})
			})
		}

		runOnMain(func() {
			progress.SetValue(float64(i + 1))
			statusLabel.SetText(fmt.Sprintf("Estimated %d/%d games...", i+1, totalGames))
		})
	}

	// Add summary at the end
	var totalSizeStr string
	if sizeUnit == "gb" {
		totalInGB := float64(totalBytes) / (1024 * 1024 * 1024)
		totalSizeStr = fmt.Sprintf("%.2f GB", totalInGB)
	} else {
		totalInMB := float64(totalBytes) / (1024 * 1024)
		totalSizeStr = fmt.Sprintf("%.2f MB", totalInMB)
	}

	summaryRows := []interface{}{
		sizeResult{"", ""},
		sizeResult{"=== Summary ===", ""},
		sizeResult{"Total Games Processed", fmt.Sprintf("%d", totalGames)},
		sizeResult{"Successful Estimations", fmt.Sprintf("%d", successCount)},
		sizeResult{"Failed Estimations", fmt.Sprintf("%d", errorCount)},
		sizeResult{"Total Estimated Size", totalSizeStr},
	}

	runOnMain(func() {
		currentItems, _ := results.Get()
		allItems := append(currentItems, summaryRows...)
		_ = results.Set(allItems)
	})
}
