package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2/data/binding"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// progressUpdater handles JSON messages from the download client
// and updates the GUI bindings.
type progressUpdater struct {
	task              *DownloadTask
	totalBytes        int64
	downloadedBytes   int64
	fileBytes         map[string]int64
	mu                sync.Mutex
	incompleteMessage []byte
}

func (pu *progressUpdater) Write(p []byte) (n int, err error) {
	pu.mu.Lock()
	defer pu.mu.Unlock()

	// Prepend any incomplete data from the previous write
	data := append(pu.incompleteMessage, p...)
	pu.incompleteMessage = nil

	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var update client.ProgressUpdate
		if err := dec.Decode(&update); err != nil {
			// It's likely we received an incomplete JSON object. Store it and wait for the next write.
			offset := int(dec.InputOffset())
			pu.incompleteMessage = data[offset:]
			break
		}

		switch update.Type {
		case "start":
			pu.totalBytes = update.OverallTotalBytes
		case "file_progress":
			// Add the difference in downloaded bytes for this specific file
			diff := update.CurrentBytes - pu.fileBytes[update.FileName]
			pu.downloadedBytes += diff
			pu.fileBytes[update.FileName] = update.CurrentBytes

			// Update overall progress
			if pu.totalBytes > 0 {
				progress := float64(pu.downloadedBytes) / float64(pu.totalBytes)
				pu.task.Progress.Set(progress)
			}
			status := fmt.Sprintf("Downloading: %s", update.FileName)
			pu.task.Status.Set(status)
		}
	}

	return len(p), nil
}

func executeDownload(authService *auth.Service, dm *DownloadManager, game db.Game,
	downloadPath, language, platformName string, extrasFlag, dlcFlag, resumeFlag,
	flattenFlag, skipPatchesFlag bool, numThreads int) {

	ctx, cancel := context.WithCancel(context.Background())
	task := &DownloadTask{
		ID:         game.ID,
		Title:      game.Title,
		Status:     binding.NewString(),
		Progress:   binding.NewFloat(),
		CancelFunc: cancel,
	}
	_ = task.Status.Set("Preparing...")
	_ = dm.AddTask(task)

	token, err := authService.RefreshToken()
	if err != nil {
		_ = task.Status.Set(fmt.Sprintf("Error: %v", err))
		return
	}

	parsedGameData, err := client.ParseGameData(game.Data)
	if err != nil {
		_ = task.Status.Set(fmt.Sprintf("Error: %v", err))
		return
	}

	updater := &progressUpdater{
		task:      task,
		fileBytes: make(map[string]int64),
	}

	err = client.DownloadGameFiles(
		ctx, token.AccessToken, parsedGameData, downloadPath, language, platformName,
		extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, numThreads,
		updater,
	)

	if err != nil {
		if errors.Is(err, context.Canceled) {
			_ = task.Status.Set("Cancelled")
		} else {
			_ = task.Status.Set(fmt.Sprintf("Error: %v", err))
		}
		return
	}

	targetDir := filepath.Join(downloadPath, client.SanitizePath(parsedGameData.Title))
	_ = task.Status.Set(fmt.Sprintf("Completed. Files in: %s", targetDir))
	_ = task.Progress.Set(1.0)
	go PlayNotificationSound()
}
