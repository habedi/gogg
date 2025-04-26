// gui/download.go
package gui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout" // Import layout package for Spacer
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// guiLogWriter struct and Write method remain the same...
type guiLogWriter struct {
	logOutput *widget.Entry
	mu        sync.Mutex
	buffer    string
}

func (w *guiLogWriter) Write(p []byte) (n int, err error) {
	// ... (implementation unchanged) ...
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buffer += string(p)
	for {
		idx := strings.IndexAny(w.buffer, "\n\r")
		if idx == -1 {
			break
		}
		line := w.buffer[:idx+1]
		w.buffer = w.buffer[idx+1:]
		cleanedLine := strings.ReplaceAll(line, "\r", "")
		cleanedLine = strings.TrimSpace(cleanedLine)
		if cleanedLine != "" {
			runOnMain(func() {
				currentText := w.logOutput.Text
				newText := currentText
				lastLineIndex := strings.LastIndex(currentText, "\n")
				lastLine := ""
				if lastLineIndex != -1 && lastLineIndex < len(currentText)-1 {
					lastLine = currentText[lastLineIndex+1:]
				}
				if lastLine != cleanedLine {
					newText += cleanedLine + "\n"
					// Optional: Trim log
					w.logOutput.SetText(newText)
				}
			})
		}
	}
	return len(p), nil
}

// DownloadUI builds a grid-based layout for downloading game files.
func DownloadUI(win fyne.Window) fyne.CanvasObject {
	var downloadCtx context.Context
	var downloadCancel context.CancelFunc

	// --- Widgets (existing - no changes needed here) ---
	gameIDLabel := widget.NewLabel("Game ID")
	gameIDEntry := widget.NewEntry()
	gameIDEntry.SetPlaceHolder("Enter a game ID")

	downloadDirLabel := widget.NewLabel("Download Path")
	downloadDirEntry := widget.NewEntry()
	downloadDirEntry.SetPlaceHolder("Enter the the path to store the game files")
	currentDir, err := os.Getwd()
	if err == nil {
		defaultDownloadPath := filepath.Join(currentDir, "games")
		downloadDirEntry.SetText(defaultDownloadPath)
	}
	browseBtn := widget.NewButton("Browse", func() {
		initialDir := downloadDirEntry.Text
		if _, statErr := os.Stat(initialDir); os.IsNotExist(statErr) {
			initialDir, _ = os.Getwd()
		}
		dirURI, _ := storage.ParseURI("file://" + initialDir)
		listableURI, _ := storage.ListerForURI(dirURI)

		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if uri != nil {
				downloadDirEntry.SetText(uri.Path())
			}
		}, win)

		if listableURI != nil {
			fd.SetLocation(listableURI)
		}
		fd.Show()
	})

	row1 := container.NewGridWithColumns(2,
		container.NewVBox(gameIDLabel, gameIDEntry),
		container.NewVBox(downloadDirLabel, container.NewBorder(nil, nil, nil, browseBtn, downloadDirEntry)),
	)

	langLabel := widget.NewLabel("Language")
	langOptions := []string{"en", "fr", "de", "es", "it", "ru", "pl", "pt-BR", "zh-Hans", "ja", "ko"}
	langSelect := widget.NewSelect(langOptions, nil)
	langSelect.SetSelected("en")

	platformLabel := widget.NewLabel("Platform")
	platformOptions := []string{"all", "windows", "mac", "linux"}
	platformSelect := widget.NewSelect(platformOptions, nil)
	platformSelect.SetSelected("windows")

	threadsLabel := widget.NewLabel("Threads")
	threadsEntry := widget.NewEntry()
	threadsEntry.SetPlaceHolder("1-20 worker threads can be used")
	threadsEntry.SetText("1") // Default to 1 thread

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

	row3 := container.NewHBox(extrasCheck, dlcCheck, resumeCheck, flattenCheck)

	// --- Download and Cancel Buttons (unchanged) ---
	downloadBtn := widget.NewButton("Download Game", nil)
	cancelBtn := widget.NewButton("Cancel Download", nil)
	cancelBtn.Hide()
	buttonStack := container.NewStack(downloadBtn, cancelBtn)
	row4 := container.NewGridWithColumns(1, buttonStack)

	// --- Form Layout (unchanged) ---
	formGrid := container.NewVBox(row1, row2, row3, row4)

	// --- Log Output Area ---
	logOutput := widget.NewMultiLineEntry()
	logOutput.SetPlaceHolder("Download logs will appear here")
	logOutput.Wrapping = fyne.TextWrapWord
	logOutput.SetMinRowsVisible(10) // Ensure it has a reasonable minimum size

	// --- Clear Log Button ---
	clearLogBtn := widget.NewButton("Clear Logs", func() {
		// This runs directly on the main GUI thread, no runOnMain needed
		logOutput.SetText("")
	})

	// --- Layout for Log Area + Clear Button ---
	// Use a Border layout: logOutput fills the center, button goes at the bottom.
	// HBox puts the button at the bottom-right using a spacer.
	logAreaWithButton := container.NewBorder(
		nil, // Top
		container.NewHBox(layout.NewSpacer(), clearLogBtn), // Bottom: Spacer pushes button to the right
		nil,       // Left
		nil,       // Right
		logOutput, // Center: logOutput fills the remaining space
	)

	// Use a vertical split to separate top (form) from bottom (log + clear button).
	split := container.NewVSplit(formGrid, logAreaWithButton) // Use logAreaWithButton here
	split.SetOffset(0.35)                                     // Adjust offset if needed

	// --- Progress Writer (unchanged) ---
	progressWriter := &guiLogWriter{logOutput: logOutput}

	// --- Button Logic (unchanged) ---
	downloadBtn.OnTapped = func() {
		// ... (validation logic unchanged) ...
		gameIDStr := strings.TrimSpace(gameIDEntry.Text)
		downloadDir := strings.TrimSpace(downloadDirEntry.Text)
		if gameIDStr == "" || downloadDir == "" {
			dialog.ShowError(fmt.Errorf("game ID and a download path are required"), win)
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

		runOnMain(func() {
			logOutput.SetText("") // Clear log on new download start
			downloadBtn.Disable()
			cancelBtn.Show()
			cancelBtn.Enable()
		})

		downloadCtx, downloadCancel = context.WithCancel(context.Background())

		go func() {
			defer func() {
				runOnMain(func() {
					downloadBtn.Enable()
					cancelBtn.Hide()
					cancelBtn.Disable()
					if downloadCancel != nil {
						downloadCancel()
						downloadCancel = nil
						downloadCtx = nil
					}
				})
			}()

			executeDownloadUI(downloadCtx, gameID, downloadDir, lang, platform, extrasFlag, dlcFlag, resumeFlag, flattenFlag, numThreads, logOutput, progressWriter)
		}()
	}

	cancelBtn.OnTapped = func() {
		runOnMain(func() {
			appendLog(logOutput, ">>> Cancellation request received...")
			cancelBtn.Disable()
		})
		if downloadCancel != nil {
			downloadCancel()
		}
	}

	return split
}

// executeDownloadUI function remains the same...
func executeDownloadUI(ctx context.Context, gameID int, downloadPath, language, platformName string, extrasFlag, dlcFlag, resumeFlag, flattenFlag bool, numThreads int, logOutput *widget.Entry, progressWriter io.Writer) {
	// ... (implementation unchanged) ...
	appendLog(logOutput, fmt.Sprintf("Starting download for game ID %d...", gameID))

	if numThreads < 1 || numThreads > 20 {
		appendLog(logOutput, "Number of threads must be between 1 and 20.")
		return
	}

	langFull, ok := gameLanguages[language]
	if !ok {
		appendLog(logOutput, "Invalid language code.")
		return
	}
	language = langFull

	token, err := client.RefreshToken()
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Failed to refresh token. Did you login? Error: %v", err))
		return
	}
	if token == nil {
		appendLog(logOutput, "Failed to get token record after refresh. Did you login?")
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
		appendLog(logOutput, fmt.Sprintf("Failed to get game by ID from DB: %v", err))
		return
	}
	if game == nil {
		appendLog(logOutput, fmt.Sprintf("Game with ID %d not found in the local catalogue.", gameID))
		return
	}

	parsedGameData, err := client.ParseGameData(game.Data)
	if err != nil {
		appendLog(logOutput, fmt.Sprintf("Failed to parse game data: %v", err))
		return
	}

	logDownloadParametersUI(
		parsedGameData, gameID, downloadPath, language, platformName,
		extrasFlag, dlcFlag, resumeFlag, flattenFlag, numThreads, logOutput,
	)

	err = client.DownloadGameFiles(
		ctx,
		token.AccessToken, parsedGameData, downloadPath, language, platformName,
		extrasFlag, dlcFlag, resumeFlag, flattenFlag, numThreads,
		progressWriter,
	)

	if errors.Is(ctx.Err(), context.Canceled) {
		appendLog(logOutput, "<<< Download cancelled.")
		return
	}

	if err != nil {
		appendLog(logOutput, fmt.Sprintf("<<< Download failed: %v", err))
		return
	}

	targetPath := filepath.Join(downloadPath, client.SanitizePath(parsedGameData.Title))
	appendLog(logOutput, fmt.Sprintf("<<< Game files downloaded successfully to: \"%s\"", targetPath))
}

// logDownloadParametersUI function remains the same...
func logDownloadParametersUI(
	game client.Game, gameID int, downloadPath, language, platformName string,
	extrasFlag, dlcFlag, resumeFlag bool, flattenFlag bool, numThreads int,
	logOutput *widget.Entry,
) {
	// ... (implementation unchanged) ...
	appendLog(logOutput, "================================= Download Parameters =====================================")
	appendLog(logOutput, fmt.Sprintf("Downloading \"%v\" (Game ID: %d) to \"%v\"", game.Title, gameID, downloadPath))
	appendLog(logOutput, fmt.Sprintf("Platform: \"%v\", Language: '%v'", platformName, language))
	appendLog(logOutput, fmt.Sprintf("Include Extras: %v, Include DLCs: %v, Resume: %v", extrasFlag, dlcFlag, resumeFlag))
	appendLog(logOutput, fmt.Sprintf("Worker Threads: %d, Flatten Directory: %v", numThreads, flattenFlag))
	appendLog(logOutput, "============================================================================================")
}
