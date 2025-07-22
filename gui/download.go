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
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

type guiLogWriter struct {
	logOutput *widget.Entry
	mu        sync.Mutex
	buffer    string
}

func (w *guiLogWriter) Write(p []byte) (n int, err error) {
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
			fyne.Do(func() {
				currentText := w.logOutput.Text
				newText := currentText
				lastLineIndex := strings.LastIndex(currentText, "\n")
				lastLine := ""
				if lastLineIndex != -1 && lastLineIndex < len(currentText)-1 {
					lastLine = currentText[lastLineIndex+1:]
				}
				if lastLine != cleanedLine {
					newText += cleanedLine + "\n"
					w.logOutput.SetText(newText)
				}
			})
		}
	}
	return len(p), nil
}

func DownloadUI(win fyne.Window) fyne.CanvasObject {
	var downloadCtx context.Context
	var downloadCancel context.CancelFunc

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
		fd.Resize(fyne.NewSize(800, 600))
		fd.SetConfirmText("Select")
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
	threadsEntry.SetText("1")

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
	skipPatchesCheck := widget.NewCheck("Skip Patches", nil)
	skipPatchesCheck.SetChecked(false)

	row3 := container.NewHBox(extrasCheck, dlcCheck, resumeCheck, flattenCheck, skipPatchesCheck)

	downloadBtn := widget.NewButton("Download Game", nil)
	cancelBtn := widget.NewButton("Cancel Download", nil)
	cancelBtn.Hide()
	buttonStack := container.NewStack(downloadBtn, cancelBtn)
	row4 := container.NewGridWithColumns(1, buttonStack)

	formGrid := container.NewVBox(row1, row2, row3, row4)

	logOutput := widget.NewMultiLineEntry()
	logOutput.SetPlaceHolder("Download logs will appear here")
	logOutput.Wrapping = fyne.TextWrapWord
	logOutput.SetMinRowsVisible(10)

	clearLogBtn := widget.NewButton("Clear Logs", func() {
		logOutput.SetText("")
	})

	logAreaWithButton := container.NewBorder(
		nil,
		container.NewHBox(layout.NewSpacer(), clearLogBtn),
		nil,
		nil,
		logOutput,
	)

	split := container.NewVSplit(formGrid, logAreaWithButton)
	split.SetOffset(0.35)

	progressWriter := &guiLogWriter{logOutput: logOutput}

	downloadBtn.OnTapped = func() {
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
		skipPatchesFlag := skipPatchesCheck.Checked

		fyne.Do(func() {
			logOutput.SetText("")
			downloadBtn.Disable()
			cancelBtn.Show()
			cancelBtn.Enable()
		})

		downloadCtx, downloadCancel = context.WithCancel(context.Background())

		go func() {
			defer func() {
				fyne.Do(func() {
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

			executeDownloadUI(downloadCtx, gameID, downloadDir, lang, platform, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, numThreads, logOutput, progressWriter)
		}()
	}

	cancelBtn.OnTapped = func() {
		fyne.Do(func() {
			appendLog(logOutput, ">>> Cancellation request received...")
			cancelBtn.Disable()
		})
		if downloadCancel != nil {
			downloadCancel()
		}
	}

	return split
}

func executeDownloadUI(ctx context.Context, gameID int, downloadPath, language, platformName string,
	extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag bool, numThreads int, logOutput *widget.Entry,
	progressWriter io.Writer,
) {
	appendLog(logOutput, fmt.Sprintf("Starting download for game ID %d...", gameID))

	if numThreads < 1 || numThreads > 20 {
		appendLog(logOutput, "Number of threads must be between 1 and 20")
		return
	}

	langFull, ok := gameLanguages[language]
	if !ok {
		appendLog(logOutput, "Invalid language code")
		return
	}
	language = langFull

	token, err := auth.RefreshToken()
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
		appendLog(logOutput, fmt.Sprintf("Game with ID %d not found in the local catalogue", gameID))
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
		extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, numThreads,
		progressWriter,
	)

	if errors.Is(ctx.Err(), context.Canceled) {
		appendLog(logOutput, "<<< Download cancelled")
		return
	}

	if err != nil {
		appendLog(logOutput, fmt.Sprintf("<<< Download failed: %v", err))
		return
	}

	targetPath := filepath.Join(downloadPath, client.SanitizePath(parsedGameData.Title))
	appendLog(logOutput, fmt.Sprintf("<<< Game files downloaded successfully to: \"%s\"", targetPath))
}

func logDownloadParametersUI(
	game client.Game, gameID int, downloadPath, language, platformName string,
	extrasFlag, dlcFlag, resumeFlag bool, flattenFlag bool, numThreads int,
	logOutput *widget.Entry,
) {
	appendLog(logOutput, "================================= Download Parameters =====================================")
	appendLog(logOutput, fmt.Sprintf("Downloading \"%v\" (Game ID: %d) to \"%v\"", game.Title, gameID, downloadPath))
	appendLog(logOutput, fmt.Sprintf("Platform: \"%v\", Language: '%v'", platformName, language))
	appendLog(logOutput, fmt.Sprintf("Include Extras: %v, Include DLCs: %v, Resume: %v", extrasFlag, dlcFlag, resumeFlag))
	appendLog(logOutput, fmt.Sprintf("Worker Threads: %d, Flatten Directory: %v", numThreads, flattenFlag))
	appendLog(logOutput, "============================================================================================")
}
