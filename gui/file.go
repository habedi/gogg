package gui

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// Supported hash algorithms.
var hashAlgorithms = []string{"md5", "sha1", "sha256", "sha512"}

// gameLanguages maps supported language codes to their full names.
var gameLanguages = map[string]string{
	"en":      "English",
	"fr":      "French",
	"de":      "German",
	"es":      "Spanish",
	"it":      "Italian",
	"ru":      "Russian",
	"pl":      "Polish",
	"pt-BR":   "Portuguese (Brazil)",
	"zh-Hans": "Simplified Chinese",
	"ja":      "Japanese",
	"ko":      "Korean",
}

// ------------------------
// Hash File UI Components
// ------------------------

func HashUI(win fyne.Window) fyne.CanvasObject {
	dirLabel := widget.NewLabel("Path")
	dirEntry := widget.NewEntry()
	dirEntry.SetPlaceHolder("The path to the scan")

	if cwd, err := os.Getwd(); err == nil {
		dirEntry.SetText(filepath.Join(cwd, "games"))
	}

	browseBtn := widget.NewButton("Browse", func() {
		initialDir := dirEntry.Text
		if _, err := os.Stat(initialDir); os.IsNotExist(err) {
			initialDir, _ = os.Getwd()
		}
		dirURI, _ := storage.ParseURI("file://" + initialDir)
		listableURI, _ := storage.ListerForURI(dirURI)
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			runOnMainV2(func() {
				if err != nil {
					dialog.ShowError(err, win)
					return
				}
				if uri != nil {
					dirEntry.SetText(uri.Path())
				}
			})
		}, win)
		if listableURI != nil {
			fd.SetLocation(listableURI)
		}
		fd.Resize(fyne.NewSize(800, 600))
		fd.SetConfirmText("Select")
		fd.Show()
	})
	dirRow := container.NewBorder(nil, nil, dirLabel, browseBtn, dirEntry)

	algoLabel := widget.NewLabel("Hash Algorithm")
	algoSelect := widget.NewSelect(hashAlgorithms, nil)
	algoSelect.SetSelected("md5")
	algoBox := container.NewHBox(algoLabel, algoSelect)

	recursiveCheck := widget.NewCheck("Recursive", nil)
	recursiveCheck.SetChecked(true)
	saveCheck := widget.NewCheck("Save to File", nil)
	cleanCheck := widget.NewCheck("Remove Old Hash Files", nil)
	optionsBox := container.NewHBox(recursiveCheck, saveCheck, cleanCheck)

	generateBtn := widget.NewButton("Generate Hashes", nil)

	topContent := container.NewVBox(
		dirRow,
		algoBox,
		optionsBox,
		generateBtn,
	)

	logOutput := widget.NewMultiLineEntry()
	logOutput.SetPlaceHolder("Logs and results will appear here")
	logOutput.Wrapping = fyne.TextWrapWord
	logOutput.SetMinRowsVisible(8)

	clearLogBtn := widget.NewButton("Clear Logs", func() {
		logOutput.SetText("")
	})

	bottomContent := container.NewBorder(
		nil,
		container.NewHBox(layout.NewSpacer(), clearLogBtn),
		nil,
		nil,
		logOutput,
	)

	split := container.NewVSplit(topContent, bottomContent)
	split.SetOffset(0.35)

	generateBtn.OnTapped = func() {
		dir := dirEntry.Text
		if dir == "" {
			dialog.ShowError(fmt.Errorf("please select a directory"), win)
			return
		}
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			dialog.ShowError(fmt.Errorf("selected directory does not exist: %s", dir), win)
			return
		}
		algo := algoSelect.Selected
		recursive := recursiveCheck.Checked
		saveToFile := saveCheck.Checked
		clean := cleanCheck.Checked
		logOutput.SetText("")
		generateBtn.Disable()
		go func() {
			defer runOnMainV2(func() { generateBtn.Enable() })
			generateHashFilesUI(dir, algo, recursive, saveToFile, clean, win, logOutput)
		}()
	}

	return split
}

func generateHashFilesUI(dir, algo string, recursive, saveToFile, clean bool, win fyne.Window, logOutput *widget.Entry) {
	if !isValidHashAlgo(algo) {
		runOnMainV2(func() {
			appendLog(logOutput, fmt.Sprintf("ERROR: Unsupported hash algorithm: %s", algo))
			dialog.ShowError(fmt.Errorf("unsupported hash algorithm: %s", algo), win)
		})
		return
	}

	if clean {
		appendLog(logOutput, fmt.Sprintf("Cleaning old hash files from '%s'", dir))
		removeHashFilesUI(dir, recursive, logOutput)
	}

	appendLog(logOutput, fmt.Sprintf("Starting hash generation (Algo: %s, Recursive: %t, Save: %t)", algo, recursive, saveToFile))

	exclusionList := []string{
		".git", ".gitignore", ".DS_Store", "Thumbs.db",
		"desktop.ini", "*.json", "*.xml", "*.csv", "*.log", "*.txt", "*.md", "*.html", "*.htm",
		"*.md5", "*.sha1", "*.sha256", "*.sha512", "*.cksum", "*.sum", "*.sig", "*.asc", "*.gpg",
	}

	var hashFiles []string
	var hfMutex sync.Mutex

	fileChan := make(chan string, 100)
	var wg sync.WaitGroup

	numWorkers := runtime.NumCPU() - 1
	if numWorkers < 1 {
		numWorkers = 1
	}

	go func() {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				appendLog(logOutput, fmt.Sprintf("Access error: %q: %v", path, err))
				return nil
			}
			if info.IsDir() {
				if path != dir && !recursive {
					return filepath.SkipDir
				}
				return nil
			}
			excluded := false
			for _, pattern := range exclusionList {
				if matched, _ := filepath.Match(pattern, info.Name()); matched {
					excluded = true
					break
				}
			}
			if !excluded {
				for _, ha := range hashAlgorithms {
					if strings.HasSuffix(info.Name(), "."+ha) {
						excluded = true
						break
					}
				}
			}
			if !excluded {
				fileChan <- path
			}
			return nil
		})
		close(fileChan)
	}()

	var countMutex sync.Mutex
	processedCount := 0

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for path := range fileChan {
				hashVal, err := generateHash(path, algo)
				if err != nil {
					appendLog(logOutput, fmt.Sprintf("Worker %d: Error hashing %s: %v", workerID, path, err))
					continue
				}
				if saveToFile {
					hashFilePath := path + "." + algo
					if err := os.WriteFile(hashFilePath, []byte(hashVal), 0o644); err != nil {
						appendLog(logOutput, fmt.Sprintf("Worker %d: Error writing hash to %s: %v", workerID, hashFilePath, err))
					} else {
						hfMutex.Lock()
						hashFiles = append(hashFiles, hashFilePath)
						hfMutex.Unlock()
					}
				} else {
					appendLog(logOutput, fmt.Sprintf("\"%s\": %s", path, hashVal))
				}
				countMutex.Lock()
				processedCount++
				if processedCount%100 == 0 {
					appendLog(logOutput, fmt.Sprintf("Processed %d files", processedCount))
				}
				countMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	countMutex.Lock()
	finalCount := processedCount
	countMutex.Unlock()

	if saveToFile {
		appendLog(logOutput, "--- Hash Generation Complete ---")
		hfMutex.Lock()
		total := len(hashFiles)
		hfMutex.Unlock()
		appendLog(logOutput, fmt.Sprintf("Generated %d hash file(s).", total))
	} else {
		appendLog(logOutput, "--- Hash Calculation Complete ---")
		appendLog(logOutput, fmt.Sprintf("Finished processing %d files. Hashes logged above.", finalCount))
	}
}

func removeHashFilesUI(dir string, recursive bool, logOutput *widget.Entry) {
	removedCount := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			appendLog(logOutput, fmt.Sprintf("Access error during clean: %q: %v", path, err))
			return nil
		}
		if info.IsDir() {
			if path != dir && !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		for _, algo := range hashAlgorithms {
			if strings.HasSuffix(path, "."+algo) {
				if rmErr := os.Remove(path); rmErr != nil {
					appendLog(logOutput, fmt.Sprintf("Error removing %s: %v", path, rmErr))
				} else {
					appendLog(logOutput, fmt.Sprintf("Removed %s", path))
					removedCount++
				}
				break
			}
		}
		return nil
	})
	appendLog(logOutput, fmt.Sprintf("Finished cleaning old hash files in %s. Removed %d file(s).", dir, removedCount))
}

// SizeUI function remains unchanged.
func SizeUI(win fyne.Window) fyne.CanvasObject {
	gameIDEntry := widget.NewEntry()
	gameIDEntry.SetPlaceHolder("Enter a game ID")
	gameIDRow := container.NewBorder(
		widget.NewLabel("Game ID"), nil, nil, nil, gameIDEntry,
	)

	langLabel := widget.NewLabel("Language")
	langSelect := widget.NewSelect(
		func() []string {
			keys := make([]string, 0, len(gameLanguages))
			for k := range gameLanguages {
				keys = append(keys, k)
			}
			return keys
		}(),
		nil,
	)
	langSelect.SetSelected("en")

	platformLabel := widget.NewLabel("Platform")
	platformSelect := widget.NewSelect(
		[]string{"all", "windows", "mac", "linux"},
		nil,
	)
	platformSelect.SetSelected("windows")

	unitLabel := widget.NewLabel("Size Unit")
	unitSelect := widget.NewSelect([]string{"mb", "gb"}, nil)
	unitSelect.SetSelected("gb")

	extrasCheck := widget.NewCheck("Include Extras", nil)
	extrasCheck.SetChecked(true)
	dlcsCheck := widget.NewCheck("Include DLCs", nil)
	dlcsCheck.SetChecked(true)

	estimateBtn := widget.NewButton("Estimate Size", nil)
	logOutput := widget.NewMultiLineEntry()
	logOutput.SetPlaceHolder("Logs and results will appear here")
	logOutput.Wrapping = fyne.TextWrapWord
	logOutput.SetMinRowsVisible(8)

	clearLogBtn := widget.NewButton("Clear Logs", func() {
		logOutput.SetText("")
	})

	top := container.NewVBox(
		gameIDRow,
		container.NewGridWithColumns(2, langLabel, langSelect),
		container.NewGridWithColumns(2, platformLabel, platformSelect),
		container.NewGridWithColumns(2, unitLabel, unitSelect),
		container.NewHBox(extrasCheck, dlcsCheck),
		estimateBtn,
	)

	bottom := container.NewBorder(
		nil,
		container.NewHBox(layout.NewSpacer(), clearLogBtn),
		nil,
		nil,
		logOutput,
	)

	split := container.NewVSplit(top, bottom)
	split.SetOffset(0.3)

	estimateBtn.OnTapped = func() {
		logOutput.SetText("")
		go func() {
			_ = estimateStorageSizeUI(
				strings.TrimSpace(gameIDEntry.Text),
				langSelect.Selected,
				platformSelect.Selected,
				extrasCheck.Checked,
				dlcsCheck.Checked,
				unitSelect.Selected,
				win,
				logOutput,
			)
		}()
	}

	return split
}

// estimateStorageSizeUI remains unchanged.
func estimateStorageSizeUI(gameID, language, platformName string, extrasFlag, dlcFlag bool, sizeUnit string, win fyne.Window, logOutput *widget.Entry) error {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		appendLog(logOutput, "Game ID cannot be empty.")
		return fmt.Errorf("game ID cannot be empty")
	}

	sizeUnit = strings.ToLower(sizeUnit)
	if sizeUnit != "mb" && sizeUnit != "gb" {
		appendLog(logOutput, fmt.Sprintf("Invalid size unit: \"%s\". Use mb or gb.", sizeUnit))
		return fmt.Errorf("invalid size unit")
	}

	langFull, ok := gameLanguages[language]
	if !ok {
		appendLog(logOutput, "Invalid language code. Supported languages are:")
		for code, name := range gameLanguages {
			appendLog(logOutput, fmt.Sprintf("'%s' for %s", code, name))
		}
		return fmt.Errorf("invalid language code")
	}
	language = langFull

	gameIDInt, err := strconv.Atoi(gameID)
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Invalid game ID: %s", gameID))
		return err
	}

	game, err := db.GetGameByID(gameIDInt)
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Failed to retrieve game data for ID %d: %v", gameIDInt, err))
		return err
	}
	if game == nil {
		appendLog(logOutput, fmt.Sprintf("Game with ID %d not found.", gameIDInt))
		return fmt.Errorf("game not found")
	}

	var nestedData client.Game
	if err := json.Unmarshal([]byte(game.Data), &nestedData); err != nil {
		appendLog(logOutput, fmt.Sprintf("Failed to unmarshal game data for ID %d: %v", gameIDInt, err))
		return err
	}

	parseSize := func(sizeStr string) (float64, error) {
		s := strings.TrimSpace(strings.ToLower(sizeStr))
		var val float64
		var unit string
		switch {
		case strings.HasSuffix(s, " gb"):
			unit = "gb"
			_, _ = fmt.Sscanf(s, "%f gb", &val)
		case strings.HasSuffix(s, " mb"):
			unit = "mb"
			_, _ = fmt.Sscanf(s, "%f mb", &val)
		case strings.HasSuffix(s, " kb"):
			unit = "kb"
			_, _ = fmt.Sscanf(s, "%f kb", &val)
		default:
			bytesVal, err := strconv.ParseInt(s, 10, 64)
			if err == nil {
				return float64(bytesVal) / (1024 * 1024), nil
			}
			return 0, fmt.Errorf("unknown or missing size unit in '%s'", sizeStr)
		}
		switch unit {
		case "gb":
			return val * 1024, nil
		case "mb":
			return val, nil
		case "kb":
			return val / 1024, nil
		}
		return 0, nil
	}

	totalSizeMB := 0.0
	appendLog(logOutput, fmt.Sprintf("Estimating size for Game: %s (ID: %d)", nestedData.Title, gameIDInt))
	appendLog(logOutput, fmt.Sprintf("Params: Lang=%s, Platform=%s, Extras=%t, DLCs=%t", language, platformName, extrasFlag, dlcFlag))

	for _, download := range nestedData.Downloads {
		if !strings.EqualFold(download.Language, language) {
			continue
		}
		for _, pf := range []struct {
			files []client.PlatformFile
			name  string
		}{
			{download.Platforms.Windows, "windows"},
			{download.Platforms.Mac, "mac"},
			{download.Platforms.Linux, "linux"},
		} {
			if platformName != "all" && !strings.EqualFold(platformName, pf.name) {
				continue
			}
			for _, file := range pf.files {
				fileName := file.Name
				if file.ManualURL != nil && *file.ManualURL != "" {
					fileName = *file.ManualURL
				}
				size, err := parseSize(file.Size)
				if err != nil {
					appendLog(logOutput, fmt.Sprintf("WARN: Failed to parse size for %s (%s): %v", fileName, file.Size, err))
				} else {
					appendLog(logOutput, fmt.Sprintf("  Game File: %s (%s)", fileName, file.Size))
					totalSizeMB += size
				}
			}
		}
	}

	if extrasFlag {
		appendLog(logOutput, " Including Extras:")
		for _, extra := range nestedData.Extras {
			size, err := parseSize(extra.Size)
			if err != nil {
				appendLog(logOutput, fmt.Sprintf("WARN: Failed to parse extra size for %s (%s): %v", extra.Name, extra.Size, err))
			} else {
				appendLog(logOutput, fmt.Sprintf("  Extra: %s (%s)", extra.Name, extra.Size))
				totalSizeMB += size
			}
		}
	}

	if dlcFlag {
		appendLog(logOutput, " Including DLCs:")
		for _, dlc := range nestedData.DLCs {
			appendLog(logOutput, fmt.Sprintf("  DLC: %s", dlc.Title))
			for _, download := range dlc.ParsedDownloads {
				if !strings.EqualFold(download.Language, language) {
					continue
				}
				for _, pf := range []struct {
					files []client.PlatformFile
					name  string
				}{
					{download.Platforms.Windows, "windows"},
					{download.Platforms.Mac, "mac"},
					{download.Platforms.Linux, "linux"},
				} {
					if platformName != "all" && !strings.EqualFold(platformName, pf.name) {
						continue
					}
					for _, file := range pf.files {
						fileName := file.Name
						if file.ManualURL != nil && *file.ManualURL != "" {
							fileName = *file.ManualURL
						}
						size, err := parseSize(file.Size)
						if err != nil {
							appendLog(logOutput, fmt.Sprintf("WARN: Failed to parse DLC file size for %s (%s): %v", fileName, file.Size, err))
						} else {
							appendLog(logOutput, fmt.Sprintf("    DLC File: %s (%s)", fileName, file.Size))
							totalSizeMB += size
						}
					}
				}
			}
			if extrasFlag {
				for _, extra := range dlc.Extras {
					size, err := parseSize(extra.Size)
					if err != nil {
						appendLog(logOutput, fmt.Sprintf("WARN: Failed to parse DLC extra size for %s (%s): %v", extra.Name, extra.Size, err))
					} else {
						appendLog(logOutput, fmt.Sprintf("    DLC Extra: %s (%s)", extra.Name, extra.Size))
						totalSizeMB += size
					}
				}
			}
		}
	}

	appendLog(logOutput, "--- Estimation Complete ---")
	if sizeUnit == "gb" {
		appendLog(logOutput, fmt.Sprintf("Total Estimated Download Size: %.2f GB", totalSizeMB/1024))
	} else {
		appendLog(logOutput, fmt.Sprintf("Total Estimated Download Size: %.0f MB", totalSizeMB))
	}

	return nil
}

// isValidHashAlgo checks supported algorithms.
func isValidHashAlgo(algo string) bool {
	for _, a := range hashAlgorithms {
		if strings.ToLower(algo) == a {
			return true
		}
	}
	return false
}

// generateHash returns the hex‐encoded hash of the file.
func generateHash(path, algo string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var h hash.Hash
	switch strings.ToLower(algo) {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		return "", fmt.Errorf("unsupported hash algorithm: %s", algo)
	}
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
