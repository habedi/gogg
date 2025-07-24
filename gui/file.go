package gui

import (
	"context"
	"encoding/json"
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
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/hasher"
	"github.com/habedi/gogg/pkg/pool"
)

type hashResult struct {
	File string
	Hash string
}

type sizeResult struct {
	Key   string
	Value string
}

// resizableTable is a custom widget that wraps a table to make its columns resize dynamically.
type resizableTable struct {
	widget.BaseWidget
	table        *widget.Table
	hashColWidth float32
}

func newResizableTable(table *widget.Table, hashColWidth float32) *resizableTable {
	rt := &resizableTable{
		table:        table,
		hashColWidth: hashColWidth,
	}
	rt.ExtendBaseWidget(rt)
	return rt
}

func (rt *resizableTable) CreateRenderer() fyne.WidgetRenderer {
	return &resizableTableRenderer{
		resizableTable: rt,
	}
}

type resizableTableRenderer struct {
	resizableTable *resizableTable
}

func (r *resizableTableRenderer) Layout(size fyne.Size) {
	r.resizableTable.table.Resize(size)
	pathColWidth := size.Width - r.resizableTable.hashColWidth - theme.Padding()
	if pathColWidth < 200 {
		pathColWidth = 200
	}
	r.resizableTable.table.SetColumnWidth(0, pathColWidth)
	r.resizableTable.table.SetColumnWidth(1, r.resizableTable.hashColWidth)
}

func (r *resizableTableRenderer) MinSize() fyne.Size {
	return r.resizableTable.table.MinSize()
}

func (r *resizableTableRenderer) Refresh() {
	r.resizableTable.table.Refresh()
}

func (r *resizableTableRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.resizableTable.table}
}

func (r *resizableTableRenderer) Destroy() {}

func HashUI(win fyne.Window) fyne.CanvasObject {
	dirEntry := widget.NewEntry()
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

	algoSelect := widget.NewSelect(hasher.HashAlgorithms, nil)
	algoSelect.SetSelected("md5")
	threadsSelect := widget.NewSelect([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, nil)
	threadsSelect.SetSelected("4")
	recursiveCheck := widget.NewCheck("Recursive", nil)
	recursiveCheck.SetChecked(true)
	cleanCheck := widget.NewCheck("Remove Old Hash Files", nil)

	form := widget.NewForm(
		widget.NewFormItem("Directory", pathContainer),
		widget.NewFormItem("Algorithm", algoSelect),
		widget.NewFormItem("Threads", threadsSelect),
	)

	generateBtn := widget.NewButton("Generate File Hashes", nil)
	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	topContent := container.NewVBox(form, container.NewHBox(recursiveCheck, cleanCheck), generateBtn, progressBar)

	resultsData := binding.NewUntypedList()
	const hashColWidth float32 = 540

	table := widget.NewTable(
		func() (int, int) { return resultsData.Length(), 2 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			item, err := resultsData.GetValue(id.Row)
			if err != nil {
				return
			}
			res := item.(hashResult)
			label := cell.(*widget.Label)
			if id.Col == 0 {
				label.SetText(res.File)
			} else {
				label.SetText(res.Hash)
			}
		},
	)
	// Use the custom resizableTable widget
	resizableTableWidget := newResizableTable(table, hashColWidth)

	header := container.NewGridWithColumns(2,
		widget.NewLabelWithStyle("File Path (relative to directory)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Hash", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
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
			generateHashFilesUI(dir, algoSelect.Selected, recursiveCheck.Checked, cleanCheck.Checked, numThreads, resultsData, progressBar)
		}()
	}

	return container.NewBorder(topContent, nil, nil, nil, container.NewBorder(header, nil, nil, nil, resizableTableWidget))
}

func generateHashFilesUI(dir, algo string, recursive, clean bool, numThreads int, results binding.UntypedList, progress *widget.ProgressBar) {
	if clean {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if info != nil && !info.IsDir() {
				for _, a := range hasher.HashAlgorithms {
					if strings.HasSuffix(info.Name(), "."+a) {
						_ = os.Remove(path)
						break
					}
				}
			}
			return nil
		})
	}

	var filesToProcess []string
	exclusionList := []string{".*", "*.json", "*.xml", "*.csv", "*.log", "*.txt", "*.md", "*.html", "*.htm"}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || (info != nil && info.IsDir()) {
			return nil
		}
		for _, pattern := range exclusionList {
			if matched, _ := filepath.Match(pattern, info.Name()); matched {
				return nil
			}
		}
		filesToProcess = append(filesToProcess, path)
		return nil
	})

	totalFiles := len(filesToProcess)
	if totalFiles == 0 {
		return
	}
	progress.Max = float64(totalFiles)

	var processedCount atomic.Int64
	workerFunc := func(ctx context.Context, path string) error {
		hashVal, err := hasher.GenerateHash(path, algo)
		if err == nil {
			relativePath, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				relativePath = filepath.Base(path)
			}
			runOnMain(func() {
				_ = results.Append(hashResult{File: relativePath, Hash: hashVal})
			})
		}
		newCount := processedCount.Add(1)
		runOnMain(func() {
			progress.SetValue(float64(newCount))
		})
		return nil
	}

	pool.Run(context.Background(), filesToProcess, numThreads, workerFunc)
}

func SizeUI(win fyne.Window) fyne.CanvasObject {
	games, err := db.GetCatalogue()
	if err != nil {
		return widget.NewLabel("Error loading game catalogue: " + err.Error())
	}

	gameMap := make(map[string]int)
	allGameTitles := make([]string, len(games))
	for i, game := range games {
		gameMap[game.Title] = game.ID
		allGameTitles[i] = game.Title
	}
	sort.Strings(allGameTitles)

	gameSelect := widget.NewSelect(allGameTitles, nil)
	gameSelect.PlaceHolder = "Select a game..."

	filterEntry := widget.NewEntry()
	filterEntry.SetPlaceHolder("Type game title to filter...")
	filterEntry.OnChanged = func(s string) {
		s = strings.ToLower(s)
		if s == "" {
			gameSelect.Options = allGameTitles
		} else {
			filtered := make([]string, 0)
			for _, title := range allGameTitles {
				if strings.Contains(strings.ToLower(title), s) {
					filtered = append(filtered, title)
				}
			}
			gameSelect.Options = filtered
		}
		gameSelect.ClearSelected()
		gameSelect.Refresh()
	}

	langCodes := make([]string, 0, len(client.GameLanguages))
	for code := range client.GameLanguages {
		langCodes = append(langCodes, code)
	}
	sort.Strings(langCodes)
	langSelect := widget.NewSelect(langCodes, nil)
	langSelect.SetSelected("en")

	platformSelect := widget.NewSelect([]string{"all", "windows", "mac", "linux"}, nil)
	platformSelect.SetSelected("windows")
	unitSelect := widget.NewSelect([]string{"gb", "mb"}, nil)
	unitSelect.SetSelected("gb")

	extrasCheck := widget.NewCheck("Include Extras", nil)
	extrasCheck.SetChecked(true)
	dlcsCheck := widget.NewCheck("Include DLCs", nil)
	dlcsCheck.SetChecked(true)

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
		func() fyne.CanvasObject { return widget.NewLabel("Template") },
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			item, err := resultsData.GetValue(id.Row)
			if err != nil {
				return
			}
			res := item.(sizeResult)
			label := cell.(*widget.Label)
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

	return container.NewBorder(topContent, nil, nil, nil, resultsTable)
}

func estimateStorageSizeUI(gameID, languageCode, platformName string, extrasFlag, dlcFlag bool, sizeUnit string, results binding.UntypedList) {
	_ = results.Set(make([]interface{}, 0))

	langFullName, ok := client.GameLanguages[languageCode]
	if !ok {
		_ = results.Append(sizeResult{"Error", "Invalid language code."})
		return
	}

	gameIDInt, _ := strconv.Atoi(gameID)
	game, err := db.GetGameByID(gameIDInt)
	if err != nil || game == nil {
		_ = results.Append(sizeResult{"Error", "Failed to retrieve game from database."})
		return
	}

	var nestedData client.Game
	if err := json.Unmarshal([]byte(game.Data), &nestedData); err != nil {
		_ = results.Append(sizeResult{"Error", "Failed to parse game data."})
		return
	}

	totalSizeBytes, err := nestedData.EstimateStorageSize(langFullName, platformName, extrasFlag, dlcFlag)
	if err != nil {
		_ = results.Append(sizeResult{"Error", "Failed to calculate size."})
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

	rows := []interface{}{
		sizeResult{"Game", nestedData.Title},
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
