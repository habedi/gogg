// ui/download.go
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
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
	downloadDirEntry.SetPlaceHolder("Select download directory...")

	browseBtn := widget.NewButton("Browse", func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if uri != nil {
				downloadDirEntry.SetText(uri.Path())
			}
		}, win)
		fd.Show()
	})

	// This row is split into two columns:
	// Left: "Game ID" label + entry
	// Right: "Download Dir" label + (entry + Browse button)
	row1 := container.NewGridWithColumns(2,
		container.NewVBox(
			gameIDLabel,
			gameIDEntry,
		),
		container.NewVBox(
			downloadDirLabel,
			// We use a Border layout so the Browse button stays on the right
			// while the text entry expands in the middle.
			container.NewBorder(nil, nil, nil, browseBtn, downloadDirEntry),
		),
	)

	// 3) Language, Platform, Threads in one row
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

	// 4) Checkboxes
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

	// 5) Download button (row 4)
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

	// Download button logic
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

		runOnMain(func() { logOutput.SetText("") })
		downloadBtn.Disable()
		go func() {
			executeDownloadUI(
				gameID, downloadDir, lang, platform,
				extrasFlag, dlcFlag, resumeFlag, flattenFlag,
				numThreads, win, logOutput,
			)
			runOnMain(func() { downloadBtn.Enable() })
		}()
	}

	return split
}

// ShowDownloadGUI creates and displays a window for downloading game files.
func ShowDownloadGUI(a fyne.App) {
	win := a.NewWindow("Download Game Files")
	win.SetContent(DownloadUI(win))
	win.Resize(fyne.NewSize(900, 600))
	win.Show()
}

// executeDownloadUI performs the download process and logs output to logOutput.
func executeDownloadUI(
	gameID int,
	downloadPath, language, platformName string,
	extrasFlag, dlcFlag, resumeFlag, flattenFlag bool,
	numThreads int,
	win fyne.Window,
	logOutput *widget.Entry,
) {
	appendLog(logOutput, fmt.Sprintf("Starting download for game ID %d...", gameID))

	if numThreads < 1 || numThreads > 20 {
		appendLog(logOutput, "Number of threads must be between 1 and 20.")
		return
	}

	langFull, ok := gameLanguages[language]
	if !ok {
		appendLog(logOutput, "Invalid language code. Supported languages are:")
		for code, name := range gameLanguages {
			appendLog(logOutput, fmt.Sprintf("'%s' for %s", code, name))
		}
		return
	}
	language = langFull

	if _, err := client.RefreshToken(); err != nil {
		appendLog(logOutput, "Failed to refresh token. Did you login?")
		return
	}

	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		appendLog(logOutput, fmt.Sprintf("Creating download directory: %s", downloadPath))
		if err := os.MkdirAll(downloadPath, os.ModePerm); err != nil {
			appendLog(logOutput, fmt.Sprintf("Failed to create download directory: %v", err))
			return
		}
	}

	game, err := db.GetGameByID(gameID)
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Failed to get game by ID: %v", err))
		return
	} else if game == nil {
		appendLog(logOutput, "Game not found in the catalogue.")
		return
	}

	parsedGameData, err := client.ParseGameData(game.Data)
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Failed to parse game data: %v", err))
		return
	}

	user, err := db.GetTokenRecord()
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Failed to retrieve user data: %v", err))
		return
	}

	logDownloadParametersUI(
		parsedGameData, gameID, downloadPath, language, platformName,
		extrasFlag, dlcFlag, resumeFlag, flattenFlag, numThreads, logOutput,
	)

	err = client.DownloadGameFiles(
		user.AccessToken, parsedGameData, downloadPath, language, platformName,
		extrasFlag, dlcFlag, resumeFlag, flattenFlag, numThreads,
	)
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Download failed: %v", err))
		return
	}
	targetPath := filepath.Join(downloadPath, client.SanitizePath(parsedGameData.Title))
	appendLog(logOutput, fmt.Sprintf("Game files downloaded successfully to: \"%s\"", targetPath))
}

// logDownloadParametersUI logs the download parameters to the log output.
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
	appendLog(logOutput, fmt.Sprintf("Platform: \"%v\", Language: '%v'", platformName, language))
	appendLog(logOutput, fmt.Sprintf("Include Extras: %v, Include DLCs: %v, Resume: %v", extrasFlag, dlcFlag, resumeFlag))
	appendLog(logOutput, fmt.Sprintf("Worker Threads: %d, Flatten Directory: %v", numThreads, flattenFlag))
	appendLog(logOutput, "============================================================================================")
}
