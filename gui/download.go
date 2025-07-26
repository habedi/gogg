package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
)

var (
	ErrDownloadInProgress = errors.New("download already in progress")
	activeDownloads       = make(map[int]struct{})
	activeDownloadsMutex  = &sync.Mutex{}
)

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

type progressUpdater struct {
	task              *DownloadTask
	totalBytes        int64
	downloadedBytes   int64
	fileBytes         map[string]int64
	fileProgress      map[string]struct{ current, total int64 }
	mu                sync.Mutex
	incompleteMessage []byte
	lastUpdateTime    time.Time
	lastBytes         int64
	speeds            []float64
	speedAvgSize      int
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
			pu.lastUpdateTime = time.Now()
		case "file_progress":
			diff := update.CurrentBytes - pu.fileBytes[update.FileName]
			pu.downloadedBytes += diff
			pu.fileBytes[update.FileName] = update.CurrentBytes

			if pu.totalBytes > 0 {
				progress := float64(pu.downloadedBytes) / float64(pu.totalBytes)
				_ = pu.task.Progress.Set(progress)
			}
			if pu.task.State == StatePreparing {
				pu.task.State = StateDownloading
				_ = pu.task.Status.Set("Downloading files...")
			}
			pu.updateSpeedAndETA()

			pu.fileProgress[update.FileName] = struct{ current, total int64 }{update.CurrentBytes, update.TotalBytes}
			if update.CurrentBytes >= update.TotalBytes && update.TotalBytes > 0 {
				delete(pu.fileProgress, update.FileName)
			}
			pu.updateFileStatusText()
		}
	}

	return len(p), nil
}

func (pu *progressUpdater) updateSpeedAndETA() {
	now := time.Now()
	elapsed := now.Sub(pu.lastUpdateTime).Seconds()

	if elapsed < 1.0 {
		return
	}

	bytesSinceLast := pu.downloadedBytes - pu.lastBytes
	currentSpeed := float64(bytesSinceLast) / elapsed

	if pu.speedAvgSize == 0 {
		pu.speedAvgSize = 5
	}
	pu.speeds = append(pu.speeds, currentSpeed)
	if len(pu.speeds) > pu.speedAvgSize {
		pu.speeds = pu.speeds[1:]
	}

	var totalSpeed float64
	for _, s := range pu.speeds {
		totalSpeed += s
	}
	avgSpeed := totalSpeed / float64(len(pu.speeds))

	pu.lastUpdateTime = now
	pu.lastBytes = pu.downloadedBytes

	detailsStr := fmt.Sprintf("Speed: %s/s", formatBytes(int64(avgSpeed)))
	remainingBytes := pu.totalBytes - pu.downloadedBytes
	if avgSpeed > 0 && remainingBytes > 0 {
		etaSeconds := float64(remainingBytes) / avgSpeed
		duration, _ := time.ParseDuration(fmt.Sprintf("%fs", math.Round(etaSeconds)))
		detailsStr += fmt.Sprintf(" | ETA: %s", duration.Truncate(time.Second).String())
	}

	_ = pu.task.Details.Set(detailsStr)
}

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
	flattenFlag, skipPatchesFlag bool, numThreads int) error {

	activeDownloadsMutex.Lock()
	if _, exists := activeDownloads[game.ID]; exists {
		log.Warn().Int("gameID", game.ID).Msg("Download is already in progress. Ignoring new request.")
		activeDownloadsMutex.Unlock()
		return ErrDownloadInProgress
	}
	activeDownloads[game.ID] = struct{}{}
	activeDownloadsMutex.Unlock()

	go func() {
		defer func() {
			activeDownloadsMutex.Lock()
			delete(activeDownloads, game.ID)
			activeDownloadsMutex.Unlock()
			dm.PersistHistory()
		}()

		ctx, cancel := context.WithCancel(context.Background())

		parsedGameData, err := client.ParseGameData(game.Data)
		if err != nil {
			fmt.Printf("Error parsing game data for %s: %v\n", game.Title, err)
			cancel()
			return
		}
		targetDir := filepath.Join(downloadPath, client.SanitizePath(parsedGameData.Title))

		task := &DownloadTask{
			ID:           game.ID,
			InstanceID:   time.Now(),
			Title:        game.Title,
			State:        StatePreparing,
			Status:       binding.NewString(),
			Details:      binding.NewString(),
			Progress:     binding.NewFloat(),
			CancelFunc:   cancel,
			FileStatus:   binding.NewString(),
			DownloadPath: targetDir,
		}
		_ = task.Status.Set("Preparing...")
		_ = task.Details.Set("Speed: N/A | ETA: N/A")
		_ = dm.AddTask(task)

		fyne.CurrentApp().Preferences().SetString("lastUsedDownloadPath", downloadPath)

		token, err := authService.RefreshToken()
		if err != nil {
			task.State = StateError
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
				task.State = StateCancelled
				_ = task.Status.Set("Cancelled")
			} else {
				task.State = StateError
				_ = task.Status.Set(fmt.Sprintf("Error: %v", err))
			}
			_ = task.FileStatus.Set("")
			_ = task.Details.Set("")
			return
		}

		task.State = StateCompleted
		_ = task.Status.Set(fmt.Sprintf("Download completed. Files are stored in: %s", targetDir))
		_ = task.Details.Set("")
		_ = task.Progress.Set(1.0)
		_ = task.FileStatus.Set("")
		go PlayNotificationSound()
	}()

	return nil
}
