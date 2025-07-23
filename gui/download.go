package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"fyne.io/fyne/v2/data/binding"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// formatBytes converts a byte count into a human-readable string (KB, MB, GB).
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// progressUpdater handles JSON messages from the download client
// and updates the GUI bindings.
type progressUpdater struct {
	task              *DownloadTask
	totalBytes        int64
	downloadedBytes   int64
	fileBytes         map[string]int64
	fileProgress      map[string]struct{ current, total int64 }
	mu                sync.Mutex
	incompleteMessage []byte
}

func (pu *progressUpdater) Write(p []byte) (n int, err error) {
	pu.mu.Lock()
	defer pu.mu.Unlock()

	data := append(pu.incompleteMessage, p...)
	pu.incompleteMessage = nil

	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var update client.ProgressUpdate
		if err := dec.Decode(&update); err != nil {
			offset := int(dec.InputOffset())
			pu.incompleteMessage = data[offset:]
			break
		}

		switch update.Type {
		case "start":
			pu.totalBytes = update.OverallTotalBytes
		case "file_progress":
			diff := update.CurrentBytes - pu.fileBytes[update.FileName]
			pu.downloadedBytes += diff
			pu.fileBytes[update.FileName] = update.CurrentBytes

			if pu.totalBytes > 0 {
				progress := float64(pu.downloadedBytes) / float64(pu.totalBytes)
				_ = pu.task.Progress.Set(progress)
			}
			_ = pu.task.Status.Set("Downloading files...")

			// Update the map for per-file progress text
			pu.fileProgress[update.FileName] = struct{ current, total int64 }{update.CurrentBytes, update.TotalBytes}
			if update.CurrentBytes >= update.TotalBytes && update.TotalBytes > 0 {
				delete(pu.fileProgress, update.FileName)
			}
			pu.updateFileStatusText()
		}
	}

	return len(p), nil
}

// updateFileStatusText builds and sets the multi-line string for per-file progress.
func (pu *progressUpdater) updateFileStatusText() {
	if len(pu.fileProgress) == 0 {
		_ = pu.task.FileStatus.Set("")
		return
	}

	files := make([]string, 0, len(pu.fileProgress))
	for f := range pu.fileProgress {
		files = append(files, f)
	}
	sort.Strings(files)

	var sb strings.Builder
	const maxLines = 3
	for i, file := range files {
		if i >= maxLines {
			sb.WriteString(fmt.Sprintf("...and %d more files.", len(files)-maxLines))
			break
		}
		progress := pu.fileProgress[file]
		percentage := 0
		if progress.total > 0 {
			percentage = int((float64(progress.current) / float64(progress.total)) * 100)
		}
		sizeStr := fmt.Sprintf("%s/%s", formatBytes(progress.current), formatBytes(progress.total))
		sb.WriteString(fmt.Sprintf("%s: %s (%d%%)\n", file, sizeStr, percentage))
	}

	_ = pu.task.FileStatus.Set(strings.TrimSpace(sb.String()))
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
		FileStatus: binding.NewString(),
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
		task:         task,
		fileBytes:    make(map[string]int64),
		fileProgress: make(map[string]struct{ current, total int64 }),
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
		_ = task.FileStatus.Set("") // Clear per-file status on error
		return
	}

	targetDir := filepath.Join(downloadPath, client.SanitizePath(parsedGameData.Title))
	_ = task.Status.Set(fmt.Sprintf("Completed. Files in: %s", targetDir))
	_ = task.Progress.Set(1.0)
	_ = task.FileStatus.Set("") // Clear per-file status on completion
	go PlayNotificationSound()
}
