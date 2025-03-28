package ui

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

// HashUI returns a Fyne CanvasObject that lets the user pick a directory,
// choose options (hash algorithm, recursive, save, clean), and then generate
// hash files for files in the selected directory.
func HashUI(win fyne.Window) fyne.CanvasObject {
	// Directory selection.
	dirLabel := widget.NewLabel("Directory:")
	dirEntry := widget.NewEntry()
	dirEntry.SetPlaceHolder("Select directory...")
	dirEntry.Resize(fyne.NewSize(400, 30))
	browseBtn := widget.NewButton("Browse", func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if uri != nil {
				dirEntry.SetText(uri.Path())
			}
		}, win)
		fd.Show()
	})

	// Create a grid for the directory row with three columns: label, entry, and button.
	dirGrid := container.NewGridWithColumns(3, dirLabel, dirEntry, browseBtn)

	// Options: algorithm selection and checkboxes.
	algoLabel := widget.NewLabel("Algorithm:")
	algoSelect := widget.NewSelect(hashAlgorithms, nil)
	algoSelect.SetSelected("sha256")
	// Arrange algorithm label and selection in a grid with two columns.
	algoGrid := container.NewGridWithColumns(2, algoLabel, algoSelect)

	recursiveCheck := widget.NewCheck("Recursive", nil)
	recursiveCheck.SetChecked(true)
	saveCheck := widget.NewCheck("Save to File", nil)
	cleanCheck := widget.NewCheck("Clean old hash files", nil)
	// Arrange checkboxes in a grid with three columns.
	optionsGrid := container.NewGridWithColumns(3, recursiveCheck, saveCheck, cleanCheck)

	// "Generate Hashes" button.
	generateBtn := widget.NewButton("Generate Hashes", nil)

	// Arrange the above grids in a main grid layout with 4 rows.
	topGrid := container.NewGridWithRows(4, dirGrid, algoGrid, optionsGrid, generateBtn)

	// Log output area.
	logOutput := widget.NewMultiLineEntry()
	logOutput.SetPlaceHolder("Log output...")
	logOutput.Wrapping = fyne.TextWrapWord
	logOutput.SetMinRowsVisible(8)

	// Use a vertical split: grid layout on top, log output on bottom.
	split := container.NewVSplit(topGrid, logOutput)
	split.SetOffset(0.35)

	// When "Generate Hashes" is clicked, run the hashing logic.
	generateBtn.OnTapped = func() {
		dir := dirEntry.Text
		if dir == "" {
			dialog.ShowError(fmt.Errorf("please select a directory"), win)
			return
		}
		algo := algoSelect.Selected
		recursive := recursiveCheck.Checked
		saveToFile := saveCheck.Checked
		clean := cleanCheck.Checked

		runOnMain(func() { logOutput.SetText("") })
		generateBtn.Disable()
		go func() {
			generateHashFilesUI(dir, algo, recursive, saveToFile, clean, win, logOutput)
			runOnMain(func() { generateBtn.Enable() })
		}()
	}

	return split
}

// generateHashFilesUI performs the file walk, hash generation, and logging.
func generateHashFilesUI(dir, algo string, recursive, saveToFile, clean bool, win fyne.Window, logOutput *widget.Entry) {
	if !isValidHashAlgo(algo) {
		runOnMain(func() {
			dialog.ShowError(fmt.Errorf("unsupported hash algorithm: %s", algo), win)
		})
		return
	}

	if clean {
		appendLog(logOutput, fmt.Sprintf("Cleaning old hash files from %s...", dir))
		removeHashFilesUI(dir, recursive, logOutput)
	}

	exclusionList := []string{
		".git", ".gitignore", ".DS_Store", "Thumbs.db",
		"desktop.ini", "*.json", "*.xml", "*.csv", "*.log", "*.txt", "*.md", "*.html", "*.htm",
		"*.md5", "*.sha1", "*.sha256", "*.sha512", "*.cksum", "*.sum", "*.sig", "*.asc", "*.gpg",
	}

	var hashFiles []string
	fileChan := make(chan string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	numWorkers := runtime.NumCPU() - 2
	if numWorkers < 2 {
		numWorkers = 2
	}

	go func() {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				appendLog(logOutput, fmt.Sprintf("Error accessing path %q: %v", path, err))
				return err
			}
			if info.IsDir() {
				if path != dir && !recursive {
					return filepath.SkipDir
				}
				return nil
			}
			for _, pattern := range exclusionList {
				if matched, _ := filepath.Match(pattern, info.Name()); matched {
					return nil
				}
			}
			fileChan <- path
			return nil
		})
		if err != nil {
			appendLog(logOutput, fmt.Sprintf("Error walking directory: %v", err))
		}
		close(fileChan)
	}()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileChan {
				hashVal, err := generateHash(path, algo)
				if err != nil {
					appendLog(logOutput, fmt.Sprintf("Error generating hash for %s: %v", path, err))
					continue
				}
				if saveToFile {
					hashFilePath := path + "." + algo
					err = os.WriteFile(hashFilePath, []byte(hashVal), 0o644)
					if err != nil {
						appendLog(logOutput, fmt.Sprintf("Error writing hash to %s: %v", hashFilePath, err))
						continue
					}
					mu.Lock()
					hashFiles = append(hashFiles, hashFilePath)
					mu.Unlock()
				} else {
					appendLog(logOutput, fmt.Sprintf("%s hash for \"%s\": %s", algo, path, hashVal))
				}
			}
		}()
	}

	wg.Wait()

	if saveToFile {
		appendLog(logOutput, "Generated hash files:")
		for _, file := range hashFiles {
			appendLog(logOutput, file)
		}
	}
}

// removeHashFilesUI removes existing hash files in the directory.
func removeHashFilesUI(dir string, recursive bool, logOutput *widget.Entry) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			appendLog(logOutput, fmt.Sprintf("Error accessing %q: %v", path, err))
			return err
		}
		if info.IsDir() && !recursive {
			return filepath.SkipDir
		}
		for _, algo := range hashAlgorithms {
			if strings.HasSuffix(path, "."+algo) {
				if err := os.Remove(path); err != nil {
					appendLog(logOutput, fmt.Sprintf("Error removing %s: %v", path, err))
				} else {
					appendLog(logOutput, fmt.Sprintf("Removed %s", path))
				}
			}
		}
		return nil
	})
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Error removing hash files: %v", err))
	} else {
		appendLog(logOutput, fmt.Sprintf("Old hash files removed from %s", dir))
	}
}

// ------------------------
// Storage Size UI Components
// ------------------------

// SizeUI returns a UI for estimating download size based on game ID and options.
func SizeUI(win fyne.Window) fyne.CanvasObject {
	gameIDLabel := widget.NewLabel("Game ID:")
	gameIDEntry := widget.NewEntry()
	gameIDEntry.SetPlaceHolder("Enter game ID...")
	langLabel := widget.NewLabel("Language:")
	langOptions := []string{"en", "fr", "de", "es", "it", "ru", "pl", "pt-BR", "zh-Hans", "ja", "ko"}
	langSelect := widget.NewSelect(langOptions, nil)
	langSelect.SetSelected("en")
	platformLabel := widget.NewLabel("Platform:")
	platformOptions := []string{"all", "windows", "mac", "linux"}
	platformSelect := widget.NewSelect(platformOptions, nil)
	platformSelect.SetSelected("windows")
	unitLabel := widget.NewLabel("Unit:")
	unitSelect := widget.NewSelect([]string{"mb", "gb"}, nil)
	unitSelect.SetSelected("mb")

	// Arrange top fields in a grid with two columns.
	topFields := container.NewGridWithColumns(2,
		container.NewHBox(gameIDLabel, gameIDEntry),
		container.NewHBox(langLabel, langSelect),
		container.NewHBox(platformLabel, platformSelect),
		container.NewHBox(unitLabel, unitSelect),
	)

	checksRow := container.NewHBox(
		widget.NewCheck("Include Extras", nil),
		widget.NewCheck("Include DLCs", nil),
	)
	// For the checkboxes, set defaults.
	extrasCheck := checksRow.Objects[0].(*widget.Check)
	extrasCheck.SetChecked(true)
	dlcCheck := checksRow.Objects[1].(*widget.Check)
	dlcCheck.SetChecked(true)

	estimateBtn := widget.NewButton("Estimate Size", nil)

	// Log output area.
	logOutput := widget.NewMultiLineEntry()
	logOutput.SetPlaceHolder("Output...")
	logOutput.Wrapping = fyne.TextWrapWord
	logOutput.SetMinRowsVisible(8)

	topArea := container.NewVBox(topFields, checksRow, estimateBtn)
	split := container.NewVSplit(topArea, logOutput)
	split.SetOffset(0.3)

	estimateBtn.OnTapped = func() {
		runOnMain(func() { logOutput.SetText("") })
		estimateBtn.Disable()
		go func() {
			err := estimateStorageSizeUI(
				gameIDEntry.Text,
				langSelect.Selected,
				platformSelect.Selected,
				extrasCheck.Checked,
				dlcCheck.Checked,
				unitSelect.Selected,
				win,
				logOutput,
			)
			if err != nil {
				appendLog(logOutput, fmt.Sprintf("Error: %v", err))
			}
			runOnMain(func() { estimateBtn.Enable() })
		}()
	}

	return split
}

// estimateStorageSizeUI calculates and logs the total download size.
func estimateStorageSizeUI(gameID, language, platformName string, extrasFlag, dlcFlag bool, sizeUnit string, win fyne.Window, logOutput *widget.Entry) error {
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

	var totalSizeMB float64
	parseSize := func(sizeStr string) (float64, error) {
		s := strings.TrimSpace(strings.ToLower(sizeStr))
		if strings.HasSuffix(s, " gb") {
			s = strings.TrimSuffix(s, " gb")
			val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
			if err != nil {
				return 0, err
			}
			return val * 1024, nil
		} else if strings.HasSuffix(s, " mb") {
			s = strings.TrimSuffix(s, " mb")
			return strconv.ParseFloat(strings.TrimSpace(s), 64)
		}
		return 0, fmt.Errorf("unknown size unit in %s", sizeStr)
	}

	// Process downloads.
	for _, download := range nestedData.Downloads {
		if !strings.EqualFold(download.Language, language) {
			appendLog(logOutput, fmt.Sprintf("Skipping download in language %s", download.Language))
			continue
		}
		for _, pf := range []struct {
			files  []client.PlatformFile
			subDir string
		}{
			{download.Platforms.Windows, "windows"},
			{download.Platforms.Mac, "mac"},
			{download.Platforms.Linux, "linux"},
		} {
			if platformName != "all" && platformName != pf.subDir {
				appendLog(logOutput, fmt.Sprintf("Skipping platform %s", pf.subDir))
				continue
			}
			for _, file := range pf.files {
				size, err := parseSize(file.Size)
				if err != nil {
					appendLog(logOutput, fmt.Sprintf("Failed to parse file size for %s: %v", *file.ManualURL, err))
					return err
				}
				if size > 0 {
					appendLog(logOutput, fmt.Sprintf("File: %s, Size: %s", *file.ManualURL, file.Size))
					totalSizeMB += size
				}
			}
		}
	}

	// Process extras.
	if extrasFlag {
		for _, extra := range nestedData.Extras {
			size, err := parseSize(extra.Size)
			if err != nil {
				appendLog(logOutput, fmt.Sprintf("Failed to parse extra size: %v", err))
				return err
			}
			if size > 0 {
				appendLog(logOutput, fmt.Sprintf("Extra: %v, Size: %s", extra.ManualURL, extra.Size))
				totalSizeMB += size
			}
		}
	}

	// Process DLCs.
	if dlcFlag {
		for _, dlc := range nestedData.DLCs {
			for _, download := range dlc.ParsedDownloads {
				if !strings.EqualFold(download.Language, language) {
					appendLog(logOutput, fmt.Sprintf("DLC %s: Skipping language %s", dlc.Title, download.Language))
					continue
				}
				for _, pf := range []struct {
					files  []client.PlatformFile
					subDir string
				}{
					{download.Platforms.Windows, "windows"},
					{download.Platforms.Mac, "mac"},
					{download.Platforms.Linux, "linux"},
				} {
					if platformName != "all" && platformName != pf.subDir {
						appendLog(logOutput, fmt.Sprintf("DLC %s: Skipping platform %s", dlc.Title, pf.subDir))
						continue
					}
					for _, file := range pf.files {
						size, err := parseSize(file.Size)
						if err != nil {
							appendLog(logOutput, fmt.Sprintf("Failed to parse DLC file size for %s: %v", *file.ManualURL, err))
							return err
						}
						if size > 0 {
							appendLog(logOutput, fmt.Sprintf("DLC File: %s, Size: %s", *file.ManualURL, file.Size))
							totalSizeMB += size
						}
					}
				}
			}
			if extrasFlag {
				for _, extra := range dlc.Extras {
					size, err := parseSize(extra.Size)
					if err != nil {
						appendLog(logOutput, fmt.Sprintf("Failed to parse DLC extra size: %v", err))
						return err
					}
					if size > 0 {
						appendLog(logOutput, fmt.Sprintf("DLC Extra: %v, Size: %s", extra.ManualURL, extra.Size))
						totalSizeMB += size
					}
				}
			}
		}
	}

	appendLog(logOutput, fmt.Sprintf("Game title: \"%s\"", nestedData.Title))
	appendLog(logOutput, fmt.Sprintf("Download parameters: Language=%s; Platform=%s; Extras=%t; DLCs=%t", language, platformName, extrasFlag, dlcFlag))
	if sizeUnit == "gb" {
		totalSizeGB := totalSizeMB / 1024
		appendLog(logOutput, fmt.Sprintf("Total download size: %.2f GB", totalSizeGB))
	} else {
		appendLog(logOutput, fmt.Sprintf("Total download size: %.0f MB", totalSizeMB))
	}
	return nil
}

// ------------------------
// Helper Functions
// ------------------------

// isValidHashAlgo returns true if the provided algorithm is supported.
func isValidHashAlgo(algo string) bool {
	for _, a := range hashAlgorithms {
		if strings.ToLower(algo) == a {
			return true
		}
	}
	return false
}

// generateHash computes the hash of the file at filePath using the given algorithm.
func generateHash(filePath, algo string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var hashAlgo hash.Hash
	switch strings.ToLower(algo) {
	case "md5":
		hashAlgo = md5.New()
	case "sha1":
		hashAlgo = sha1.New()
	case "sha256":
		hashAlgo = sha256.New()
	case "sha512":
		hashAlgo = sha512.New()
	default:
		return "", fmt.Errorf("unsupported hash algorithm: %s", algo)
	}

	if _, err := io.Copy(hashAlgo, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hashAlgo.Sum(nil)), nil
}

// runOnMain schedules fn to run on the main thread if possible; otherwise, it calls fn directly.
func runOnMain(fn func()) {
	if driver, ok := fyne.CurrentApp().Driver().(interface{ RunOnMain(func()) }); ok {
		driver.RunOnMain(fn)
	} else {
		fn()
	}
}

// appendLog appends a message to the log output widget on the main thread.
// It also trims the log if it exceeds a maximum length.
func appendLog(logOutput *widget.Entry, msg string) {
	const maxLogLength = 10000 // maximum characters to keep
	runOnMain(func() {
		current := logOutput.Text
		newText := current + msg + "\n"
		if len(newText) > maxLogLength {
			// Trim the beginning of the log so that only the last maxLogLength characters remain.
			newText = newText[len(newText)-maxLogLength:]
		}
		logOutput.SetText(newText)
	})
}
