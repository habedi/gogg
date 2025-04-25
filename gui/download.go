// ui/download.go
package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
)

// DownloadUI builds a grid-based layout for downloading game files.
func DownloadUI(win fyne.Window) fyne.CanvasObject {
	// 1) Game ID widgets
	gameIDLabel := widget.NewLabel("Game ID:")
	gameIDEntry := widget.NewEntry()
	gameIDEntry.SetPlaceHolder("Enter game ID...")

	// 2) Download Dir widgets
	downloadDirLabel := widget.NewLabel("Download Dir:")
	downloadDirEntry := widget.NewEntry()
	downloadDirEntry.SetPlaceHolder("Select download directory...") // Placeholder is still useful

	// --- Calculate and set default download directory ---
	currentDir, err := os.Getwd()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get current working directory for default download path")
		// If we can't get the CWD, the field will just be empty initially
	} else {
		defaultDownloadPath := filepath.Join(currentDir, "games")
		downloadDirEntry.SetText(defaultDownloadPath) // Set the default text
	}
	// ----------------------------------------------------

	browseBtn := widget.NewButton("Browse", func() {
		// Pre-populate the folder dialog with the current value if it's a valid dir
		initialDir := downloadDirEntry.Text
		if _, statErr := os.Stat(initialDir); os.IsNotExist(statErr) {
			// If dir doesn't exist, maybe start browse from CWD or parent of default
			initialDir, _ = os.Getwd() // Fallback to CWD if default path doesn't exist
		}
		dirURI, _ := storage.ParseURI("file://" + initialDir) // Convert path to URI for dialog
		listableURI, _ := storage.ListerForURI(dirURI)        // Get Lister interface for dialog

		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if uri != nil {
				downloadDirEntry.SetText(uri.Path()) // Use uri.Path() for file paths
			}
		}, win)

		if listableURI != nil {
			fd.SetLocation(listableURI) // Set starting location for the dialog
		}
		fd.Show()
	})

	// Layout for Row 1 remains the same
	row1 := container.NewGridWithColumns(2,
		container.NewVBox(
			gameIDLabel,
			gameIDEntry,
		),
		container.NewVBox(
			downloadDirLabel,
			container.NewBorder(nil, nil, nil, browseBtn, downloadDirEntry),
		),
	)

	// Rows 2, 3, 4 remain the same
	langLabel := widget.NewLabel("Language:")
	langOptions := []string{"en", "fr", "de", "es", "it", "ru", "pl", "pt-BR", "zh-Hans", "ja", "ko"}
	langSelect := widget.NewSelect(langOptions, nil)
	langSelect.SetSelected("en")

	platformLabel := widget.NewLabel("Platform:")
	platformOptions := []string{"all", "windows", "mac", "linux"}
	platformSelect := widget.NewSelect(platformOptions, nil)
	platformSelect.SetSelected("windows")

	threadsLabel := widget.NewLabel("Threads:")
	threadsEntry := widget.NewEntry()
	threadsEntry.SetPlaceHolder("5")
	threadsEntry.SetText("5")

	row2 := container.NewGridWithColumns(3,
		container.NewVBox(langLabel, langSelect),
		container.NewVBox(platformLabel, platformSelect),
		container.NewVBox(threadsLabel, threadsEntry),
	)

	extrasCheck := widget.NewCheck("Include Extras", nil)
	extrasCheck.SetChecked(true)
	dlcCheck := widget.NewCheck("Include DLCs", nil)
	dlcCheck.SetChecked(true)
	resumeCheck := widget.NewCheck("Resume Download", nil)
	resumeCheck.SetChecked(true)
	flattenCheck := widget.NewCheck("Flatten Structure", nil)
	flattenCheck.SetChecked(true)

	row3 := container.NewHBox(
		extrasCheck,
		dlcCheck,
		resumeCheck,
		flattenCheck,
	)

	downloadBtn := widget.NewButton("Download Game", nil)
	row4 := container.NewGridWithColumns(1, downloadBtn)

	// Combine all rows in a vertical box (the top portion)
	formGrid := container.NewVBox(
		row1,
		row2,
		row3,
		row4,
	)

	// Multi-line log output area (the bottom portion)
	logOutput := widget.NewMultiLineEntry()
	logOutput.SetPlaceHolder("Download log output...")
	logOutput.Wrapping = fyne.TextWrapWord
	logOutput.SetMinRowsVisible(10)

	// Use a vertical split to separate top (form) from bottom (log).
	split := container.NewVSplit(formGrid, logOutput)
	split.SetOffset(0.35) // ~35% for form, ~65% for log

	// Download button logic remains the same
	downloadBtn.OnTapped = func() {
		gameIDStr := gameIDEntry.Text
		downloadDir := downloadDirEntry.Text
		if gameIDStr == "" || downloadDir == "" {
			dialog.ShowError(fmt.Errorf("game ID and download directory are required"), win)
			return
		}
		gameID, err := strconv.Atoi(gameIDStr)
		if err != nil || gameID <= 0 {
			dialog.ShowError(fmt.Errorf("invalid game ID"), win)
			return
		}
		lang := langSelect.Selected
		platform := platformSelect.Selected
		numThreads, err := strconv.Atoi(threadsEntry.Text)
		if err != nil || numThreads < 1 || numThreads > 20 {
			dialog.ShowError(fmt.Errorf("number of threads must be between 1 and 20"), win)
			return
		}
		extrasFlag := extrasCheck.Checked
		dlcFlag := dlcCheck.Checked
		resumeFlag := resumeCheck.Checked
		flattenFlag := flattenCheck.Checked

		// Clear log and disable button while download runs
		runOnMain(func() { logOutput.SetText("") }) // Ensure UI update runs on main thread
		downloadBtn.Disable()
		go func() {
			executeDownloadUI(
				gameID, downloadDir, lang, platform,
				extrasFlag, dlcFlag, resumeFlag, flattenFlag,
				numThreads, win, logOutput,
			)
			runOnMain(func() { downloadBtn.Enable() }) // Ensure UI update runs on main thread
		}()
	}

	return split
}

// ShowDownloadGUI function remains the same...
func ShowDownloadGUI(a fyne.App) {
	// This function is likely not used if GUI is launched via cmd/gui.go
	// but kept here for completeness or potential direct GUI launch.
	win := a.NewWindow("Download Game Files")
	win.SetContent(DownloadUI(win))
	win.Resize(fyne.NewSize(900, 600))
	win.Show()
}

// executeDownloadUI function remains the same...
func executeDownloadUI(
	gameID int,
	downloadPath, language, platformName string,
	extrasFlag, dlcFlag, resumeFlag, flattenFlag bool,
	numThreads int,
	win fyne.Window,
	logOutput *widget.Entry,
) {
	appendLog(logOutput, fmt.Sprintf("Starting download for game ID %d...", gameID)) // Use appendLog helper

	if numThreads < 1 || numThreads > 20 {
		appendLog(logOutput, "Number of threads must be between 1 and 20.")
		return
	}

	// Validate language using the map from file.go (or move map to shared place)
	langFull, ok := gameLanguages[language]
	if !ok {
		logMsg := "Invalid language code. Supported languages are:\n"
		for code, name := range gameLanguages {
			logMsg += fmt.Sprintf("'%s' for %s\n", code, name)
		}
		appendLog(logOutput, logMsg)
		return
	}
	language = langFull // Use the full language name for the client call

	// Check token
	token, err := client.RefreshToken()
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Failed to refresh token. Did you login? Error: %v", err))
		return
	}
	if token == nil {
		appendLog(logOutput, "Failed to get token record after refresh. Did you login?")
		return
	}

	// Check if download path exists, create if not
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		appendLog(logOutput, fmt.Sprintf("Creating download directory: %s", downloadPath))
		if err := os.MkdirAll(downloadPath, os.ModePerm); err != nil {
			appendLog(logOutput, fmt.Sprintf("Failed to create download directory: %v", err))
			return
		}
	}

	// Fetch game data from DB
	game, err := db.GetGameByID(gameID)
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Failed to get game by ID from DB: %v", err))
		return
	}
	if game == nil {
		appendLog(logOutput, fmt.Sprintf("Game with ID %d not found in the local catalogue.", gameID))
		return
	}

	// Parse game data
	parsedGameData, err := client.ParseGameData(game.Data)
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Failed to parse game data: %v", err))
		return
	}

	// Log parameters
	logDownloadParametersUI(
		parsedGameData, gameID, downloadPath, language, platformName,
		extrasFlag, dlcFlag, resumeFlag, flattenFlag, numThreads, logOutput,
	)

	// Execute download
	err = client.DownloadGameFiles(
		token.AccessToken, parsedGameData, downloadPath, language, platformName,
		extrasFlag, dlcFlag, resumeFlag, flattenFlag, numThreads,
	)
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Download failed: %v", err))
		// Even if download fails, the button should be re-enabled in the calling goroutine
		return
	}

	// Success message
	targetPath := filepath.Join(downloadPath, client.SanitizePath(parsedGameData.Title))
	appendLog(logOutput, fmt.Sprintf("Game files downloaded successfully to: \"%s\"", targetPath))
}

// logDownloadParametersUI function remains the same...
func logDownloadParametersUI(
	game client.Game,
	gameID int,
	downloadPath, language, platformName string,
	extrasFlag, dlcFlag, resumeFlag, flattenFlag bool,
	numThreads int,
	logOutput *widget.Entry,
) {
	appendLog(logOutput, "================================= Download Parameters =====================================")
	appendLog(logOutput, fmt.Sprintf("Downloading \"%v\" (Game ID: %d) to \"%v\"", game.Title, gameID, downloadPath))
	appendLog(logOutput, fmt.Sprintf("Platform: \"%v\", Language: '%v'", platformName, language)) // Use full language name here
	appendLog(logOutput, fmt.Sprintf("Include Extras: %v, Include DLCs: %v, Resume: %v", extrasFlag, dlcFlag, resumeFlag))
	appendLog(logOutput, fmt.Sprintf("Worker Threads: %d, Flatten Directory: %v", numThreads, flattenFlag))
	appendLog(logOutput, "============================================================================================")
}
