package gui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// isGameDownloaded checks if a game has been successfully downloaded based on download history
func isGameDownloaded(dm *DownloadManager, gameID int) bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	allTasks, err := dm.Tasks.Get()
	if err != nil {
		return false
	}

	for _, taskRaw := range allTasks {
		task := taskRaw.(*DownloadTask)
		if task.ID == gameID && task.State() == StateCompleted {
			return true
		}
	}
	return false
}

// getLastCompletedDownloadDir returns the download directory for the most recent completed download of a game, if any.
func getLastCompletedDownloadDir(dm *DownloadManager, gameID int) (string, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	allTasks, err := dm.Tasks.Get()
	if err != nil {
		return "", false
	}

	var (
		latestPath string
	)

	// Fallback: use time.Time from InstanceID, but we cannot compare with fyne.Time; instead compare via UnixNano
	var latestUnix int64 = -1
	for _, taskRaw := range allTasks {
		task := taskRaw.(*DownloadTask)
		if task.ID != gameID || task.State() != StateCompleted {
			continue
		}
		if t := task.InstanceID.UnixNano(); t > latestUnix {
			latestUnix = t
			latestPath = task.DownloadPath
		}
	}
	if latestUnix < 0 || latestPath == "" {
		return "", false
	}
	return latestPath, true
}

// readDownloadedMetadata loads the metadata.json stored alongside a completed download, if present.
func readDownloadedMetadata(dm *DownloadManager, gameID int) (*client.Game, error) {
	path, ok := getLastCompletedDownloadDir(dm, gameID)
	if !ok {
		return nil, fmt.Errorf("no completed download found")
	}
	metaPath := path + string(os.PathSeparator) + "metadata.json"
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var g client.Game
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// getGameDownloadDirectory says where a game's files landed, from the download
// history first and the remembered download root as a fallback.
func getGameDownloadDirectory(dm *DownloadManager, game db.Game) (string, bool) {
	if path, ok := getLastCompletedDownloadDir(dm, game.ID); ok {
		return path, true
	}
	// The same accessor the download form uses, so a path typed but not yet
	// downloaded to is still found here rather than only the last one an
	// actual download wrote.
	root := rememberedDownloadPath(fyne.CurrentApp().Preferences())
	if root == "" {
		return "", false
	}
	candidate := filepath.Join(root, client.SanitizePath(game.Title))
	if _, err := os.Stat(filepath.Join(candidate, "metadata.json")); err == nil {
		return candidate, true
	}
	return "", false
}

func readDownloadInfo(downloadDir string) (language, platform string) {
	infoPath := filepath.Join(downloadDir, "download_info.json")
	b, err := os.ReadFile(infoPath)
	if err != nil {
		return "", ""
	}
	var info struct {
		Language string `json:"language"`
		Platform string `json:"platform"`
	}
	if json.Unmarshal(b, &info) != nil {
		return "", ""
	}
	return info.Language, info.Platform
}
