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

	copyBtn := widget.NewButtonWithIcon("Copy All (CSV)", theme.ContentCopyIcon(), func() {
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

	gameSelect := widget.NewSelect(nil, nil)
	gameSelect.PlaceHolder = "Loading games..."

	refreshGameList := func() {
		games, err := db.GetCatalogue()
		if err != nil {
			gameSelect.PlaceHolder = "Error loading games"
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

		gameSelect.Options = allGameTitles
		gameSelect.PlaceHolder = fmt.Sprintf("%d games available...", len(allGameTitles))
		gameSelect.Refresh()
	}

	listener := binding.NewDataListener(func() {
		runOnMain(refreshGameList)
	})
	catalogueUpdated.AddListener(listener)

	refreshGameList()

	filterEntry := widget.NewEntry()
	filterEntry.SetPlaceHolder("Type game title to filter...")
	clearFilterBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		filterEntry.SetText("")
	})
	filterEntry.ActionItem = clearFilterBtn
	clearFilterBtn.Hide()

	filterEntry.OnChanged = func(s string) {
		s = strings.ToLower(s)
		var newOptions []string

		if s == "" {
			newOptions = allGameTitles
			clearFilterBtn.Hide()
		} else {
			filtered := make([]string, 0)
			for _, title := range allGameTitles {
				if strings.Contains(strings.ToLower(title), s) {
					filtered = append(filtered, title)
				}
			}
			newOptions = filtered
			clearFilterBtn.Show()
		}
		gameSelect.Options = newOptions
		gameSelect.PlaceHolder = fmt.Sprintf("%d games available...", len(newOptions))
		gameSelect.ClearSelected()
		gameSelect.Refresh()
	}

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
		widget.NewFormItem("Filter", filterEntry),
		widget.NewFormItem("Game", gameSelect),
		widget.NewFormItem("Language", langSelect),
		widget.NewFormItem("Platform", platformSelect),
		widget.NewFormItem("Unit", unitSelect),
	)

	estimateBtn := widget.NewButton("Estimate Size", nil)
	topContent := container.NewVBox(form, container.NewHBox(extrasCheck, dlcsCheck), estimateBtn)

	resultsData := binding.NewUntypedList()
	resultsTable := widget.NewTable(
		func() (int, int) { return resultsData.Length(), 2 },
		func() fyne.CanvasObject { return NewCopyableLabel("Template") },
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
	resultsTable.SetColumnWidth(0, 150)
	resultsTable.SetColumnWidth(1, 400)

	estimateBtn.OnTapped = func() {
		selectedGame := gameSelect.Selected
		if selectedGame == "" {
			dialog.ShowError(errors.New("please select a game"), win)
			return
		}
		gameID := gameMap[selectedGame]

		go estimateStorageSizeUI(
			strconv.Itoa(gameID),
			langSelect.Selected, platformSelect.Selected,
			extrasCheck.Checked, dlcsCheck.Checked,
			unitSelect.Selected, resultsData,
		)
	}

	clearBtn := widget.NewButtonWithIcon("Clear Results", theme.DeleteIcon(), func() {
		_ = resultsData.Set(make([]interface{}, 0))
	})

	copyBtn := widget.NewButtonWithIcon("Copy All (CSV)", theme.ContentCopyIcon(), func() {
		items, _ := resultsData.Get()
		if len(items) == 0 {
			fyne.CurrentApp().SendNotification(fyne.NewNotification("Gogg", "Nothing to copy."))
			return
		}

		var sb strings.Builder
		writer := csv.NewWriter(&sb)
		_ = writer.Write([]string{"Parameter", "Value"}) // Header

		for _, item := range items {
			res := item.(sizeResult)
			_ = writer.Write([]string{res.Key, res.Value})
		}
		writer.Flush()

		fyne.CurrentApp().Clipboard().SetContent(sb.String())
		fyne.CurrentApp().SendNotification(fyne.NewNotification("Gogg", "Size estimation results copied to clipboard."))
	})

	bottomBar := container.NewHBox(layout.NewSpacer(), clearBtn, copyBtn)

	return container.NewBorder(topContent, bottomBar, nil, nil, resultsTable)
}

func estimateStorageSizeUI(gameIDStr, languageCode, platformName string, extrasFlag, dlcFlag bool, sizeUnit string, results binding.UntypedList) {
	_ = results.Set(make([]interface{}, 0))

	gameID, err := strconv.Atoi(gameIDStr)
	if err != nil {
		_ = results.Append(sizeResult{"Error", "Invalid game ID."})
		return
	}

	params := operations.EstimationParams{
		LanguageCode:  languageCode,
		PlatformName:  platformName,
		IncludeExtras: extrasFlag,
		IncludeDLCs:   dlcFlag,
	}

	totalSizeBytes, gameData, err := operations.EstimateGameSize(gameID, params)
	if err != nil {
		_ = results.Append(sizeResult{"Error", err.Error()})
		return
	}

	var sizeStr string
	if sizeUnit == "gb" {
		sizeInGB := float64(totalSizeBytes) / (1024 * 1024 * 1024)
		sizeStr = fmt.Sprintf("%.2f GB", sizeInGB)
	} else {
		sizeInMB := float64(totalSizeBytes) / (1024 * 1024)
		sizeStr = fmt.Sprintf("%.2f MB", sizeInMB)
	}

	boolToStr := func(b bool) string {
		if b {
			return "Yes"
		}
		return "No"
	}

	langFullName := client.GameLanguages[languageCode]
	rows := []interface{}{
		sizeResult{"Game", gameData.Title},
		sizeResult{"Platform", platformName},
		sizeResult{"Language", langFullName},
		sizeResult{"Extras Included", boolToStr(extrasFlag)},
		sizeResult{"DLCs Included", boolToStr(dlcFlag)},
		sizeResult{"Estimated Size", sizeStr},
	}
	runOnMain(func() {
		_ = results.Set(rows)
	})
}
