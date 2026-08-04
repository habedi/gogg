package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"github.com/habedi/gogg/client"
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
			if pu.task.State() == StatePreparing {
				pu.task.SetState(StateDownloading)
				_ = pu.task.Status.Set("Downloading files...")
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
// the aggregate line above the download list.
func (pu *progressUpdater) publishTotals() {
	pu.task.SetProgressBytes(pu.downloadedBytes, pu.totalBytes, int64(pu.averageSpeed()))
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
	const maxLines = 2
	const maxFilenameLen = 40

	for i, file := range files {
		if i >= maxLines {
			fmt.Fprintf(&sb, "...and %d more files", len(files)-maxLines)
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
	activeDownloadsMutex.Lock()
	if _, exists := activeDownloads[q.game.ID]; exists {
		log.Warn().Int("gameID", q.game.ID).Msg("Download is already in progress. Ignoring new request.")
		activeDownloadsMutex.Unlock()
		return ErrDownloadInProgress
	}
	activeDownloads[q.game.ID] = struct{}{}
	activeDownloadsMutex.Unlock()

	releaseSlot := func() {
		activeDownloadsMutex.Lock()
		delete(activeDownloads, q.game.ID)
		activeDownloadsMutex.Unlock()
	}

	parsedGameData, err := client.ParseGameData(q.game.Data)
	if err != nil {
		releaseSlot()
		return fmt.Errorf("failed to parse game data for %s: %w", q.game.Title, err)
	}

	var targetDir string
	if q.rommLayoutFlag {
		plat := strings.ToLower(q.platformName)
		if plat == "all" { // show root for mixed
			targetDir = q.downloadPath
		} else {
			targetDir = filepath.Join(q.downloadPath, plat, client.SanitizePath(parsedGameData.Title))
		}
	} else {
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
	_ = task.Details.Set("Speed: N/A | ETA: N/A")
	// Registered before returning so that the queue counts this download as
	// active right away instead of once the goroutine below gets scheduled.
	_ = dm.AddTask(task)

	go func() {
		defer func() {
			cancel()
			releaseSlot()
			dm.PersistHistory()
			// Attempt to start queued downloads if slots free
			go dm.startNextIfAvailable()
		}()

		fyne.CurrentApp().Preferences().SetString("lastUsedDownloadPath", q.downloadPath)

		token, err := q.authService.RefreshTokenCtx(ctx)
		if err != nil {
			task.SetState(StateError)
			_ = task.Status.Set(fmt.Sprintf("Error: %v", err))
			return
		}

		updater := &progressUpdater{
			task:         task,
			dm:           dm,
			fileBytes:    make(map[string]int64),
			fileProgress: make(map[string]struct{ current, total int64 }),
		}

		err = client.DownloadGameFiles(
			ctx, token.AccessToken, parsedGameData, q.downloadPath, q.language, q.platformName,
			q.extrasFlag, q.dlcFlag, q.resumeFlag, q.flattenFlag, q.skipPatchesFlag, q.rommLayoutFlag,
			q.numThreads, updater,
		)

		if err != nil {
			if errors.Is(err, context.Canceled) {
				task.SetState(StateCancelled)
				_ = task.Status.Set("Cancelled")
			} else {
				task.SetState(StateError)
				_ = task.Status.Set(fmt.Sprintf("Error: %v", err))
			}
			_ = task.FileStatus.Set("")
			_ = task.Details.Set("")
			return
		}

		task.SetState(StateCompleted)
		_ = task.Status.Set(fmt.Sprintf("Download completed. Files are stored in: %s", targetDir))
		_ = task.Details.Set("")
		_ = task.Progress.Set(1.0)
		_ = task.FileStatus.Set("")
		go PlayNotificationSound()
		// Persist download info for future update checks.
		info := struct {
			Language    string `json:"language"`
			Platform    string `json:"platform"`
			Extras      bool   `json:"extras"`
			DLCs        bool   `json:"dlcs"`
			SkipPatches bool   `json:"skipPatches"`
			Flatten     bool   `json:"flatten"`
			Resume      bool   `json:"resume"`
			Threads     int    `json:"threads"`
		}{
			Language:    q.language,
			Platform:    q.platformName,
			Extras:      q.extrasFlag,
			DLCs:        q.dlcFlag,
			SkipPatches: q.skipPatchesFlag,
			Flatten:     q.flattenFlag,
			Resume:      q.resumeFlag,
			Threads:     q.numThreads,
		}
		if data, mErr := json.MarshalIndent(info, "", "  "); mErr == nil {
			_ = os.MkdirAll(targetDir, 0755)
			_ = os.WriteFile(filepath.Join(targetDir, "download_info.json"), data, 0644)
		}

		if q.keepLatestFlag {
			if err := guiPruneOldVersions(q.downloadPath, parsedGameData.Title, q.rommLayoutFlag, q.platformName); err != nil {
				log.Warn().Err(err).Msg("Failed to prune old versions (GUI)")
			}
		}
	}()

	return nil
}

var guiVersionPattern = regexp.MustCompile(`^(?P<prefix>.*?)(?P<ver>\d+(?:\.\d+)+)(?P<suffix>\.[^.]+)$`)

func guiParseVersion(filename string) (prefix string, verSlice []int, suffix string, ok bool) {
	m := guiVersionPattern.FindStringSubmatch(filename)
	if m == nil {
		return "", nil, "", false
	}
	prefix = m[1]
	suffix = m[3]
	parts := strings.Split(m[2], ".")
	for _, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return "", nil, "", false
		}
		verSlice = append(verSlice, v)
	}
	return prefix, verSlice, suffix, true
}

// guiInstallerGroup identifies files that are versions of the same installer.
// See installerGroup in cmd/download.go for why the directory and extension
// are part of the identity.
type guiInstallerGroup struct {
	dir    string
	prefix string
	suffix string
}

func guiCompareVersions(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		va, vb := 0, 0
		if i < len(a) {
			va = a[i]
		}
		if i < len(b) {
			vb = b[i]
		}
		if va > vb {
			return 1
		}
		if va < vb {
			return -1
		}
	}
	return 0
}

func guiPruneOldVersions(rootPath, title string, romm bool, platformName string) error {
	// Determine roots to scan
	var roots []string
	if romm {
		plats := []string{"windows", "mac", "linux"}
		if strings.ToLower(platformName) != "all" {
			plats = []string{strings.ToLower(platformName)}
		}
		for _, p := range plats {
			roots = append(roots, filepath.Join(rootPath, p, client.SanitizePath(title)))
		}
	} else {
		roots = []string{filepath.Join(rootPath, client.SanitizePath(title))}
	}
	extAllowed := map[string]struct{}{".exe": {}, ".bin": {}, ".dmg": {}, ".sh": {}, ".zip": {}, ".tar.gz": {}, ".rar": {}}
	for _, root := range roots {
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		filesByGroup := map[guiInstallerGroup][]string{}
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			name := info.Name()
			ext := filepath.Ext(name)
			if strings.HasSuffix(name, ".tar.gz") {
				ext = ".tar.gz"
			}
			if _, ok := extAllowed[ext]; !ok {
				return nil
			}
			prefix, _, suffix, ok := guiParseVersion(name)
			if !ok {
				return nil
			}
			group := guiInstallerGroup{dir: filepath.Dir(path), prefix: prefix, suffix: suffix}
			filesByGroup[group] = append(filesByGroup[group], path)
			return nil
		})
		for _, paths := range filesByGroup {
			// find the latest file among paths
			var best string
			var bestVer []int
			for _, p := range paths {
				name := filepath.Base(p)
				_, ver, _, ok := guiParseVersion(name)
				if !ok {
					continue
				}
				if best == "" || guiCompareVersions(ver, bestVer) == 1 {
					best = p
					bestVer = ver
				}
			}
			for _, p := range paths {
				if p != best {
					_ = os.Remove(p)
				}
			}
		}
	}
	return nil
}
