package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"github.com/habedi/gogg/client"
	"github.com/rs/zerolog/log"
)

// fileStatusLines is how many transfers a download card lists before it
// summarises the rest.
const fileStatusLines = 3

var ErrDownloadInProgress = errors.New("download already in progress")

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
	dm                *DownloadManager
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

	// A download of several languages or platforms runs in passes, and each
	// pass owns a slice of the one bar: base is where this pass starts, span
	// how much of the bar it may fill. Zero span means the whole bar.
	progressBase float64
	progressSpan float64
	// The header counts bytes, and it counts across every pass, not one at a
	// time: overallBase is the bytes the passes before this one already
	// brought, and overallTotal the size of the whole job. Zero overallTotal
	// means a single pass, where the pass total is the whole of it.
	overallBase  int64
	overallTotal int64
	// finishing notes that every byte of this pass has landed.
	finishing bool
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
				span := pu.progressSpan
				if span == 0 {
					span = 1
				}
				fraction := float64(pu.downloadedBytes) / float64(pu.totalBytes)
				_ = pu.task.Progress.Set(pu.progressBase + fraction*span)
			}
			if pu.task.State() == StatePreparing {
				pu.task.SetState(StateDownloading)
				_ = pu.task.Status.Set("Downloading files...")
			}
			// Every byte has landed but the call has not returned: files are
			// being renamed into place. Said, or the pause reads as a hang.
			if pu.totalBytes > 0 {
				finishing := pu.downloadedBytes >= pu.totalBytes
				if finishing != pu.finishing {
					pu.finishing = finishing
					if finishing {
						_ = pu.task.Status.Set("Finishing up...")
					} else {
						_ = pu.task.Status.Set("Downloading files...")
					}
				}
			}
			pu.updateSpeedAndETA()
			pu.publishTotals()

			pu.fileProgress[update.FileName] = struct{ current, total int64 }{update.CurrentBytes, update.TotalBytes}
			if update.CurrentBytes >= update.TotalBytes && update.TotalBytes > 0 {
				delete(pu.fileProgress, update.FileName)
			}
			pu.updateFileStatusText()
		}
	}

	return len(p), nil
}

// publishTotals mirrors this download's progress onto the task and refreshes
// the aggregate line above the download list. When the job runs in passes, it
// reports the bytes and the total for the whole job rather than the pass in
// hand, so the header counts up once instead of resetting each pass.
func (pu *progressUpdater) publishTotals() {
	downloaded, total := pu.downloadedBytes, pu.totalBytes
	if pu.overallTotal > 0 {
		downloaded, total = pu.overallBase+pu.downloadedBytes, pu.overallTotal
	}
	pu.task.SetProgressBytes(downloaded, total, int64(pu.averageSpeed()))
	if pu.dm != nil {
		pu.dm.refreshTotals()
	}
}

// averageSpeed is the smoothed transfer rate in bytes per second.
func (pu *progressUpdater) averageSpeed() float64 {
	if len(pu.speeds) == 0 {
		return 0
	}
	var total float64
	for _, speed := range pu.speeds {
		total += speed
	}
	return total / float64(len(pu.speeds))
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

	avgSpeed := pu.averageSpeed()

	pu.lastUpdateTime = now
	pu.lastBytes = pu.downloadedBytes

	remaining := pu.totalBytes - pu.downloadedBytes
	if pu.overallTotal > 0 {
		remaining = pu.overallTotal - (pu.overallBase + pu.downloadedBytes)
	}
	_ = pu.task.Details.Set(transferSummary(avgSpeed, remaining))
}

// transferSummary is the line under a running download: how fast it is going
// and how long is left. It is worded like the line above the download list, so
// the same facts do not read as two different things.
func transferSummary(speed float64, remaining int64) string {
	if speed <= 0 {
		return ""
	}

	summary := fmt.Sprintf("%s/s", formatBytes(int64(speed)))
	if remaining > 0 {
		eta := time.Duration(float64(remaining)/speed) * time.Second
		summary += " · ETA " + eta.Truncate(time.Second).String()
	}
	return summary
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
	const maxFilenameLen = 40

	for i, file := range files {
		if i >= fileStatusLines {
			fmt.Fprintf(&sb, "...and %d more files", len(files)-fileStatusLines)
			break
		}

		displayName := file
		if len(displayName) > maxFilenameLen {
			displayName = "..." + displayName[len(displayName)-maxFilenameLen+3:]
		}

		progress := pu.fileProgress[file]
		percentage := 0
		if progress.total > 0 {
			percentage = int((float64(progress.current) / float64(progress.total)) * 100)
		}
		sizeStr := fmt.Sprintf("%s/%s", formatBytes(progress.current), formatBytes(progress.total))
		fmt.Fprintf(&sb, "%s: %s (%d%%)\n", displayName, sizeStr, percentage)
	}

	_ = pu.task.FileStatus.Set(strings.TrimSpace(sb.String()))
}

// executeDownload starts a download described by q and registers it with dm.
func executeDownload(dm *DownloadManager, q queuedDownload) error {
	if !dm.acquireSlot(q.game.ID) {
		log.Warn().Int("gameID", q.game.ID).Msg("Download is already in progress. Ignoring new request.")
		return ErrDownloadInProgress
	}

	releaseSlot := func() { dm.releaseSlot(q.game.ID) }

	parsedGameData, err := client.ParseGameData(q.game.Data)
	if err != nil {
		releaseSlot()
		return fmt.Errorf("failed to parse game data for %s: %w", q.game.Title, err)
	}
	parsedGameData.ID = q.game.ID

	var targetDir string
	switch {
	case q.lutrisLayoutFlag:
		targetDir = filepath.Join(q.downloadPath, client.LutrisSlug(parsedGameData.Title), "gog")
	case q.rommLayoutFlag:
		plat := client.RomMPlatform(q.platformName)
		if plat == "all" { // show root for mixed
			targetDir = q.downloadPath
		} else {
			targetDir = filepath.Join(q.downloadPath, plat, client.SanitizePath(parsedGameData.Title))
		}
	default:
		targetDir = filepath.Join(q.downloadPath, client.SanitizePath(parsedGameData.Title))
	}

	ctx, cancel := context.WithCancel(context.Background())

	task := &DownloadTask{
		ID:           q.game.ID,
		InstanceID:   time.Now(),
		Title:        q.game.Title,
		request:      q,
		Status:       binding.NewString(),
		Details:      binding.NewString(),
		Progress:     binding.NewFloat(),
		CancelFunc:   cancel,
		FileStatus:   binding.NewString(),
		DownloadPath: targetDir,
	}
	task.SetState(StatePreparing)
	_ = task.Status.Set("Preparing...")
	// Registered before returning so that the queue counts this download as
	// active right away instead of once the goroutine below gets scheduled.
	_ = dm.AddTask(task)
	// Written to the history now, not only when it ends, so a crash mid
	// download still leaves an interrupted record to retry from next time.
	dm.PersistHistory()

	go func() {
		defer func() {
			cancel()
			releaseSlot()
			dm.PersistHistory()
			// Attempt to start queued downloads if slots free
			go dm.startNextIfAvailable()
		}()

		// Both keys are written so the form's remembered path and the
		// legacy one an older gogg stored cannot drift apart; the form and
		// the history lookup read them through one accessor.
		prefs := fyne.CurrentApp().Preferences()
		prefs.SetString("lastUsedDownloadPath", q.downloadPath)
		prefs.SetString("downloadForm.path", q.downloadPath)

		token, err := q.authService.RefreshTokenCtx(ctx)
		if err != nil {
			task.SetState(StateError)
			_ = task.Status.Set(fmt.Sprintf("Error: %v", err))
			announceIfLast(dm, StateError, q.game.Title)
			return
		}

		// A download of several languages or platforms runs as passes over the
		// same call, each owning its slice of the one progress bar. Files two
		// passes share are skipped by the second, so nothing is fetched twice.
		languages := q.languages
		if len(languages) == 0 {
			languages = []string{q.language}
		}
		platforms := q.platforms
		if len(platforms) == 0 {
			platforms = []string{q.platformName}
		}

		passes := len(languages) * len(platforms)

		// The whole job's size is worked out up front, and each pass's own
		// size with it, so the header can count bytes across every pass
		// rather than resetting when a pass ends. A size that cannot be
		// estimated leaves overallTotal zero, and the header falls back to
		// counting the pass in hand.
		var overallTotal int64
		passSizes := make([]int64, 0, passes)
		for _, language := range languages {
			for _, platform := range platforms {
				size, estErr := parsedGameData.EstimateStorageSize(language, platform, q.extrasFlag, q.dlcFlag)
				if estErr != nil {
					overallTotal = 0
					passSizes = nil
					break
				}
				passSizes = append(passSizes, size)
				overallTotal += size
			}
			if passSizes == nil {
				break
			}
		}

		pass := 0
		var overallBase int64
		for _, language := range languages {
			for _, platform := range platforms {
				updater := &progressUpdater{
					task:         task,
					dm:           dm,
					fileBytes:    make(map[string]int64),
					fileProgress: make(map[string]struct{ current, total int64 }),
					progressBase: float64(pass) / float64(passes),
					progressSpan: 1 / float64(passes),
					overallBase:  overallBase,
					overallTotal: overallTotal,
				}
				if overallTotal > 0 {
					overallBase += passSizes[pass]
				}
				err = client.DownloadGameFiles(
					ctx, token.AccessToken, parsedGameData, q.downloadPath,
					client.DownloadOptions{
						Language: language, Platform: platform,
						Extras: q.extrasFlag, DLCs: q.dlcFlag, Resume: q.resumeFlag,
						Flatten: q.flattenFlag, SkipPatches: q.skipPatchesFlag,
						RomMLayout: q.rommLayoutFlag, LutrisLayout: q.lutrisLayoutFlag,
						Threads:     q.numThreads,
						Connections: q.connections,
					}, updater,
				)
				if err != nil {
					break
				}
				pass++
			}
			if err != nil {
				break
			}
		}

		if err != nil {
			if errors.Is(err, context.Canceled) {
				if task.pausing.Load() {
					task.SetState(StatePaused)
					_ = task.Status.Set("Paused. What has arrived stays for the resume.")
					_ = task.FileStatus.Set("")
					_ = task.Details.Set("")
					announceIfLast(dm, StatePaused, q.game.Title)
					return
				}
				task.SetState(StateCancelled)
				_ = task.Status.Set("Cancelled")
			} else {
				task.SetState(StateError)
				_ = task.Status.Set(fmt.Sprintf("Error: %v", err))
			}
			_ = task.FileStatus.Set("")
			_ = task.Details.Set("")
			announceIfLast(dm, task.State(), q.game.Title)
			return
		}

		task.SetState(StateCompleted)
		_ = task.Status.Set(fmt.Sprintf("Download completed. Files are stored in: %s", targetDir))
		_ = task.Details.Set("")
		_ = task.Progress.Set(1.0)
		_ = task.FileStatus.Set("")
		announceIfLast(dm, StateCompleted, q.game.Title)
		// Persist download info for future update checks.
		info := struct {
			Language    string   `json:"language"`
			Platform    string   `json:"platform"`
			Languages   []string `json:"languages,omitempty"`
			Platforms   []string `json:"platforms,omitempty"`
			Extras      bool     `json:"extras"`
			DLCs        bool     `json:"dlcs"`
			SkipPatches bool     `json:"skipPatches"`
			Flatten     bool     `json:"flatten"`
			Resume      bool     `json:"resume"`
			Threads     int      `json:"threads"`
			Connections int      `json:"connections,omitempty"`
		}{
			Language:    q.language,
			Platform:    q.platformName,
			Languages:   q.languages,
			Platforms:   q.platforms,
			Extras:      q.extrasFlag,
			DLCs:        q.dlcFlag,
			SkipPatches: q.skipPatchesFlag,
			Flatten:     q.flattenFlag,
			Resume:      q.resumeFlag,
			Threads:     q.numThreads,
			Connections: q.connections,
		}
		if data, mErr := json.MarshalIndent(info, "", "  "); mErr == nil {
			_ = os.MkdirAll(targetDir, 0755)
			_ = os.WriteFile(filepath.Join(targetDir, "download_info.json"), data, 0644)
		}

		if q.keepLatestFlag {
			removed, pruneErr := client.PruneOldInstallerVersions(q.downloadPath, parsedGameData.Title,
				client.DownloadOptions{RomMLayout: q.rommLayoutFlag, LutrisLayout: q.lutrisLayoutFlag, Platform: q.platformName})
			if pruneErr != nil {
				log.Warn().Err(pruneErr).Msg("Failed to prune old versions (GUI)")
			}
			if len(removed) > 0 {
				log.Info().Strs("files", removed).Msg("Removed older installer versions")
				_ = task.Status.Set(fmt.Sprintf(
					"Download completed. Files are stored in: %s. Removed %d older installer %s.",
					targetDir, len(removed), filesWord(len(removed))))
			}
		}
	}()

	return nil
}

// announceIfLast plays the sound and shows the notification once the last
// in-flight download has landed, speaking for the whole batch.
func announceIfLast(dm *DownloadManager, state int, title string) {
	done, failed, last := dm.noteFinished(state)
	if !last {
		return
	}
	go PlayNotificationSound()
	notifyBatchFinished(done, failed, title)
}
