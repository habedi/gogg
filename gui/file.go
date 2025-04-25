// gui/file.go
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
	"unicode/utf8"

	"github.com/rs/zerolog/log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout" // Import layout
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	// "github.com/rs/zerolog/log"
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
	dirEntry.SetPlaceHolder("Select directory...") // Placeholder remains useful

	// --- Calculate and set default directory ---
	currentDir, err := os.Getwd()
	if err != nil {
		// Optional: Log warning if needed
		log.Warn().Err(err).Msg("Failed to get current working directory for default hash path")
		// Field will remain empty if CWD fails
	} else {
		// Use the same default as the download tab: ./games
		defaultHashPath := filepath.Join(currentDir, "games")
		dirEntry.SetText(defaultHashPath) // Set the default text
	}
	// -----------------------------------------

	browseBtn := widget.NewButton("Browse", func() {
		// Pre-populate the folder dialog with the current value if it's a valid dir
		initialDir := dirEntry.Text // Use the current text in the entry
		if _, statErr := os.Stat(initialDir); os.IsNotExist(statErr) {
			// If dir doesn't exist, maybe start browse from CWD or parent of default
			initialDir, _ = os.Getwd() // Fallback to CWD if default/current path doesn't exist
		}
		dirURI, _ := storage.ParseURI("file://" + initialDir) // Convert path to URI for dialog
		listableURI, _ := storage.ListerForURI(dirURI)        // Get Lister interface for dialog

		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if uri != nil {
				dirEntry.SetText(uri.Path()) // Use uri.Path() for file paths
			}
		}, win)

		if listableURI != nil {
			fd.SetLocation(listableURI) // Set starting location for the dialog
		}
		fd.Show()
	})
	// Using NewBorder allows the entry field to expand while keeping the button fixed size.
	dirRow := container.NewBorder(nil, nil, dirLabel, browseBtn, dirEntry)

	// Options: algorithm selection and checkboxes.
	algoLabel := widget.NewLabel("Algorithm:")
	// Default changed back to md5 as per the provided code
	algoSelect := widget.NewSelect(hashAlgorithms, nil)
	algoSelect.SetSelected("md5")
	algoBox := container.NewHBox(algoLabel, algoSelect)

	recursiveCheck := widget.NewCheck("Recursive", nil)
	recursiveCheck.SetChecked(true)
	saveCheck := widget.NewCheck("Save to File", nil)
	cleanCheck := widget.NewCheck("Clean old hash files", nil)
	optionsBox := container.NewHBox(recursiveCheck, saveCheck, cleanCheck)

	// "Generate Hashes" button.
	generateBtn := widget.NewButton("Generate Hashes", nil)

	// Arrange the UI elements vertically
	topContent := container.NewVBox(
		dirRow,
		algoBox,
		optionsBox,
		generateBtn,
	)

	// Log output area (single area for all output in this version).
	logOutput := widget.NewMultiLineEntry()
	logOutput.SetPlaceHolder("Log output...")
	logOutput.Wrapping = fyne.TextWrapWord
	logOutput.SetMinRowsVisible(8) // Keep a reasonable size

	// --- Add Clear Log Button ---
	clearLogBtn := widget.NewButton("Clear Log", func() {
		// Action to clear the logOutput text entry
		logOutput.SetText("")
	})

	// --- Combine Log Output and Clear Button ---
	// Use a Border layout: logOutput fills center, button at the bottom-right.
	bottomContent := container.NewBorder(
		nil, // Top
		container.NewHBox(layout.NewSpacer(), clearLogBtn), // Bottom: Spacer pushes button right
		nil,       // Left
		nil,       // Right
		logOutput, // Center: log output area
	)

	// Use a vertical split: Form controls on top, Log+Clear button below.
	split := container.NewVSplit(topContent, bottomContent) // Use bottomContent here
	split.SetOffset(0.35)                                   // Adjust offset as needed

	// --- Generate Button Logic ---
	generateBtn.OnTapped = func() {
		dir := dirEntry.Text
		if dir == "" {
			dialog.ShowError(fmt.Errorf("please select a directory"), win)
			return
		}
		// Make sure directory exists before proceeding
		if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
			dialog.ShowError(fmt.Errorf("selected directory does not exist: %s", dir), win)
			return
		}

		algo := algoSelect.Selected
		recursive := recursiveCheck.Checked
		saveToFile := saveCheck.Checked
		clean := cleanCheck.Checked

		// Clear log before starting and disable button
		runOnMain(func() { logOutput.SetText("") })
		generateBtn.Disable()

		// Run hashing in background
		go func() {
			// Ensure button is re-enabled when done
			defer runOnMain(func() { generateBtn.Enable() })
			// Call the original function that logs everything to logOutput
			generateHashFilesUI(dir, algo, recursive, saveToFile, clean, win, logOutput)
		}()
	}

	return split
}

// generateHashFilesUI logs results directly to logOutput if saveToFile is false.
// (Signature matches the one expected by the HashUI above)
func generateHashFilesUI(dir, algo string, recursive, saveToFile, clean bool, win fyne.Window, logOutput *widget.Entry) {
	if !isValidHashAlgo(algo) {
		runOnMain(func() {
			// Use appendLog for consistency, even for errors shown in dialog
			appendLog(logOutput, fmt.Sprintf("ERROR: Unsupported hash algorithm: %s", algo))
			dialog.ShowError(fmt.Errorf("unsupported hash algorithm: %s", algo), win)
		})
		return
	}

	if clean {
		appendLog(logOutput, fmt.Sprintf("Cleaning old hash files from %s...", dir))
		removeHashFilesUI(dir, recursive, logOutput) // Pass logOutput for cleaning messages
	}

	appendLog(logOutput, fmt.Sprintf("Starting hash generation (Algo: %s, Recursive: %t, Save: %t)...", algo, recursive, saveToFile))

	exclusionList := []string{
		".git", ".gitignore", ".DS_Store", "Thumbs.db",
		"desktop.ini", "*.json", "*.xml", "*.csv", "*.log", "*.txt", "*.md", "*.html", "*.htm",
		"*.md5", "*.sha1", "*.sha256", "*.sha512", "*.cksum", "*.sum", "*.sig", "*.asc", "*.gpg",
	}

	var hashFiles []string // Only used if saveToFile is true
	var hfMutex sync.Mutex

	fileChan := make(chan string, 100)
	var wg sync.WaitGroup

	numWorkers := runtime.NumCPU() - 1
	if numWorkers < 1 {
		numWorkers = 1
	}

	// Start walker goroutine
	go func() {
		walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				appendLog(logOutput, fmt.Sprintf("Access error: %q: %v", path, err))
				return nil // Continue if possible
			}
			if info.IsDir() {
				if path != dir && !recursive {
					return filepath.SkipDir
				}
				return nil
			}
			// Exclusion logic...
			fileName := info.Name()
			excluded := false
			for _, pattern := range exclusionList {
				matched, _ := filepath.Match(pattern, fileName)
				if matched {
					excluded = true
					break
				}
				for _, ha := range hashAlgorithms {
					if strings.HasSuffix(fileName, "."+ha) {
						excluded = true
						break
					}
				}
				if excluded {
					break
				}
			}
			if excluded {
				return nil
			}

			fileChan <- path
			return nil
		})
		if walkErr != nil {
			appendLog(logOutput, fmt.Sprintf("Error walking directory: %v", walkErr))
		}
		close(fileChan)
	}()

	processedCount := 0
	var countMutex sync.Mutex

	// Start worker goroutines
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
					// content := fmt.Sprintf("\"%s\": %s", path, hashVal)
					content := fmt.Sprintf("%s", hashVal)
					err = os.WriteFile(hashFilePath, []byte(content), 0o644)
					if err != nil {
						appendLog(logOutput, fmt.Sprintf("Worker %d: Error writing hash to %s: %v", workerID, hashFilePath, err))
						continue
					}
					hfMutex.Lock()
					hashFiles = append(hashFiles, hashFilePath)
					hfMutex.Unlock()
				} else {
					// Log the hash result directly to the logOutput widget
					appendLog(logOutput, fmt.Sprintf("\"%s\": %s", path, hashVal))
				}

				// Increment processed count and log progress
				countMutex.Lock()
				processedCount++
				if processedCount%100 == 0 {
					appendLog(logOutput, fmt.Sprintf("Processed %d files...", processedCount))
				}
				countMutex.Unlock()
			}
		}(i)
	}

	wg.Wait() // Wait for all workers to finish

	// Final status message
	finalProcessedCount := 0
	countMutex.Lock()
	finalProcessedCount = processedCount
	countMutex.Unlock()

	if saveToFile {
		appendLog(logOutput, "--- Hash Generation Complete ---")
		hfMutex.Lock()
		numGenerated := len(hashFiles)
		hfMutex.Unlock()
		if numGenerated > 0 {
			appendLog(logOutput, fmt.Sprintf("Generated %d hash file(s).", numGenerated))
			// Optionally list generated files (can be long)
			// hfMutex.Lock(); for _, f := range hashFiles { appendLog(logOutput, f) }; hfMutex.Unlock()
		} else {
			appendLog(logOutput, fmt.Sprintf("Finished processing %d files. No hash files were generated.", finalProcessedCount))
		}
	} else {
		appendLog(logOutput, "--- Hash Calculation Complete ---")
		appendLog(logOutput, fmt.Sprintf("Finished processing %d files. Hashes logged above.", finalProcessedCount))
	}
}

// removeHashFilesUI logs messages to the provided logOutput widget.
func removeHashFilesUI(dir string, recursive bool, logOutput *widget.Entry) {
	removedCount := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			appendLog(logOutput, fmt.Sprintf("Access error during clean: %q: %v", path, err))
			return nil // Continue if possible
		}
		if info.IsDir() {
			if path != dir && !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		// Process files
		for _, algo := range hashAlgorithms {
			if strings.HasSuffix(path, "."+algo) {
				if err := os.Remove(path); err != nil {
					appendLog(logOutput, fmt.Sprintf("Error removing %s: %v", path, err))
				} else {
					appendLog(logOutput, fmt.Sprintf("Removed %s", path))
					removedCount++
				}
				break // Move to next file
			}
		}
		return nil
	})
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Error during hash file removal: %v", err))
	} else {
		appendLog(logOutput, fmt.Sprintf("Finished cleaning old hash files in %s. Removed %d file(s).", dir, removedCount))
	}
}

// SizeUI function remains the same...
func SizeUI(win fyne.Window) fyne.CanvasObject {
	gameIDEntry := widget.NewEntry()
	gameIDEntry.SetPlaceHolder("Enter game ID...")
	gameIDRow := container.NewBorder(
		widget.NewLabel("Game ID:"), nil, nil, nil, gameIDEntry,
	)

	langLabel := widget.NewLabel("Language:")
	langSelect := widget.NewSelect(
		[]string{"en", "fr", "de", "es", "it", "ru", "pl", "pt-BR", "zh-Hans", "ja", "ko"},
		nil,
	)
	langSelect.SetSelected("en")

	platformLabel := widget.NewLabel("Platform:")
	platformSelect := widget.NewSelect(
		[]string{"all", "windows", "mac", "linux"},
		nil,
	)
	platformSelect.SetSelected("windows")

	unitLabel := widget.NewLabel("Unit:")
	unitSelect := widget.NewSelect(
		[]string{"mb", "gb"},
		nil,
	)
	unitSelect.SetSelected("gb")

	extrasCheck := widget.NewCheck("Include Extras", nil)
	extrasCheck.SetChecked(true)
	dlcsCheck := widget.NewCheck("Include DLCs", nil)
	dlcsCheck.SetChecked(true)

	estimateBtn := widget.NewButton("Estimate Size", nil)
	logOutput := widget.NewMultiLineEntry()

	top := container.NewVBox(
		gameIDRow,
		container.NewGridWithColumns(2, langLabel, langSelect),
		container.NewGridWithColumns(2, platformLabel, platformSelect),
		container.NewGridWithColumns(2, unitLabel, unitSelect),
		container.NewHBox(extrasCheck, dlcsCheck),
		estimateBtn,
	)

	split := container.NewVSplit(top, logOutput)
	split.SetOffset(0.3)

	estimateBtn.OnTapped = func() {
		logOutput.SetText("")
		go estimateStorageSizeUI(
			gameIDEntry.Text,
			langSelect.Selected,
			platformSelect.Selected,
			extrasCheck.Checked,
			dlcsCheck.Checked,
			unitSelect.Selected,
			win,
			logOutput,
		)
	}

	return split
}

// estimateStorageSizeUI function remains the same...
func estimateStorageSizeUI(gameID, language, platformName string, extrasFlag, dlcFlag bool, sizeUnit string, win fyne.Window, logOutput *widget.Entry) error {

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
	language = langFull // Use full name

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
		valStr := ""
		unit := ""

		if strings.HasSuffix(s, " gb") {
			valStr = strings.TrimSuffix(s, " gb")
			unit = "gb"
		} else if strings.HasSuffix(s, " mb") {
			valStr = strings.TrimSuffix(s, " mb")
			unit = "mb"
		} else if strings.HasSuffix(s, " kb") {
			valStr = strings.TrimSuffix(s, " kb")
			unit = "kb"
		} else {
			// Attempt to parse as bytes if no unit
			bytesVal, err := strconv.ParseInt(s, 10, 64)
			if err == nil {
				return float64(bytesVal) / (1024 * 1024), nil // Convert bytes to MB
			}
			return 0, fmt.Errorf("unknown or missing size unit in '%s'", sizeStr)
		}

		val, err := strconv.ParseFloat(strings.TrimSpace(valStr), 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse size value '%s': %w", valStr, err)
		}

		switch unit {
		case "gb":
			return val * 1024, nil
		case "mb":
			return val, nil
		case "kb":
			return val / 1024, nil
		default:
			return 0, fmt.Errorf("unexpected unit %s", unit) // Should not happen
		}
	}

	appendLog(logOutput, fmt.Sprintf("Estimating size for Game: %s (ID: %d)", nestedData.Title, gameIDInt))
	appendLog(logOutput, fmt.Sprintf("Params: Lang=%s, Platform=%s, Extras=%t, DLCs=%t", language, platformName, extrasFlag, dlcFlag))

	// Process downloads
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
				fileName := file.Name // Use Name field which should be present
				if file.ManualURL != nil && *file.ManualURL != "" {
					// Use ManualURL for logging if available
					fileName = *file.ManualURL
				}
				size, err := parseSize(file.Size)
				if err != nil {
					appendLog(logOutput, fmt.Sprintf("WARN: Failed to parse size for %s (%s): %v", fileName, file.Size, err))
					// Continue calculation even if one size fails
				} else if size > 0 {
					appendLog(logOutput, fmt.Sprintf("  Game File: %s (%s)", fileName, file.Size))
					totalSizeMB += size
				}
			}
		}
	}

	// Process extras
	if extrasFlag {
		appendLog(logOutput, " Including Extras:")
		for _, extra := range nestedData.Extras {
			size, err := parseSize(extra.Size)
			if err != nil {
				appendLog(logOutput, fmt.Sprintf("WARN: Failed to parse extra size for %s (%s): %v", extra.Name, extra.Size, err))
			} else if size > 0 {
				appendLog(logOutput, fmt.Sprintf("  Extra: %s (%s)", extra.Name, extra.Size))
				totalSizeMB += size
			}
		}
	}

	// Process DLCs
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
						} else if size > 0 {
							appendLog(logOutput, fmt.Sprintf("    DLC File: %s (%s)", fileName, file.Size))
							totalSizeMB += size
						}
					}
				}
			}
			// DLC Extras
			if extrasFlag {
				for _, extra := range dlc.Extras {
					size, err := parseSize(extra.Size)
					if err != nil {
						appendLog(logOutput, fmt.Sprintf("WARN: Failed to parse DLC extra size for %s (%s): %v", extra.Name, extra.Size, err))
					} else if size > 0 {
						appendLog(logOutput, fmt.Sprintf("    DLC Extra: %s (%s)", extra.Name, extra.Size))
						totalSizeMB += size
					}
				}
			}
		}
	}

	appendLog(logOutput, "--- Estimation Complete ---")
	if sizeUnit == "gb" {
		totalSizeGB := totalSizeMB / 1024
		appendLog(logOutput, fmt.Sprintf("Total Estimated Download Size: %.2f GB", totalSizeGB))
	} else {
		appendLog(logOutput, fmt.Sprintf("Total Estimated Download Size: %.0f MB", totalSizeMB))
	}
	return nil
}

// Helper functions remain the same...
func isValidHashAlgo(algo string) bool {
	// ... (implementation unchanged) ...
	for _, a := range hashAlgorithms {
		if strings.ToLower(algo) == a {
			return true
		}
	}
	return false
}

func generateHash(filePath, algo string) (string, error) {
	// ... (implementation unchanged) ...
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

func runOnMain(fn func()) {
	// ... (implementation unchanged) ...
	if driver, ok := fyne.CurrentApp().Driver().(interface{ RunOnMain(func()) }); ok {
		driver.RunOnMain(fn)
	} else {
		fn()
	}
}

// appendLog appends a message, ensuring rune safety and better trimming.
// MODIFIED: Rune-safe trimming and trimming to the next newline.
func appendLog(logOutput *widget.Entry, msg string) {
	// Use a rune count limit for safety, might need adjustment.
	// ~8000 runes is often less than 10000 bytes, but safer.
	const maxLogLengthRunes = 8000

	runOnMain(func() {
		currentText := logOutput.Text
		// Use a strings.Builder for efficiency if appending frequently
		var builder strings.Builder
		builder.Grow(len(currentText) + len(msg) + 1) // Pre-allocate roughly
		builder.WriteString(currentText)
		builder.WriteString(msg)
		builder.WriteString("\n")
		newText := builder.String()

		// Check actual rune count for trimming decision
		if utf8.RuneCountInString(newText) > maxLogLengthRunes {
			// Convert the full new text to runes for safe processing
			runes := []rune(newText)
			runeCount := len(runes)

			// Calculate how many runes to keep
			runesToKeep := maxLogLengthRunes
			// Calculate the starting rune index
			startRuneIndex := runeCount - runesToKeep

			// Ensure start index is valid
			if startRuneIndex < 0 {
				startRuneIndex = 0
			}
			if startRuneIndex >= runeCount {
				// This means maxLogLengthRunes is 0 or negative, clear text?
				logOutput.SetText("")
				return
			}

			// Slice the runes to get the tail end
			trimmedRunes := runes[startRuneIndex:]

			// Convert back to string *before* finding newline
			trimmedText := string(trimmedRunes)

			// Find the first newline in the potentially trimmed text
			// This ensures we don't start with a partial line.
			firstNewlineIndex := strings.Index(trimmedText, "\n")

			if firstNewlineIndex != -1 && firstNewlineIndex+1 < len(trimmedText) {
				// If a newline is found, take the substring *after* it
				newText = trimmedText[firstNewlineIndex+1:]
			} else {
				// If no newline found in the trimmed section (unlikely for large logs)
				// or if the newline is the very last character,
				// use the rune-trimmed text as is.
				newText = trimmedText
			}
		}
		// Set the final processed text
		logOutput.SetText(newText)
	})
} // End of appendLog
