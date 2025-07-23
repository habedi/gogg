package gui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2/data/binding"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

type progressWriter struct {
	task *DownloadTask
}

func (pw *progressWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		_ = pw.task.Status.Set(msg)
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
	_ = task.Status.Set("Starting...")
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

	pWriter := &progressWriter{task: task}

	err = client.DownloadGameFiles(
		ctx, token.AccessToken, parsedGameData, downloadPath, language, platformName,
		extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, numThreads,
		pWriter,
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
