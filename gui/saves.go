package gui

import (
	"context"
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// cloudSaveBackuper is what the backup button needs from the cloud client.
// An interface so tests can hand in a fake instead of four stub servers.
type cloudSaveBackuper interface {
	BackupCloudSaves(ctx context.Context, refreshToken string, gameID int, platform, outputDir string) (client.BackupResult, error)
}

// newCloudSaveClient is a test seam; production always talks to GOG.
var newCloudSaveClient = func() cloudSaveBackuper { return client.NewCloudSaveClient() }

// backupSaves fetches one game's cloud saves into outputDir in the
// background and says how it went through a notification, the same channel
// finished downloads use. Backup only reads from the cloud, so the button
// carries no risk worth a confirmation dialog.
func backupSaves(win fyne.Window, authService *auth.Service, game db.Game, outputDir string) {
	go func() {
		token, err := authService.RefreshTokenCtx(context.Background())
		if err != nil {
			fyne.Do(func() {
				showErrorDialog(win, "Cannot back up saves", errors.New("not logged in to GOG; please login first"))
			})
			return
		}

		result, err := newCloudSaveClient().BackupCloudSaves(context.Background(), token.RefreshToken, game.ID, "all", outputDir)
		fyne.Do(func() {
			switch {
			case errors.Is(err, client.ErrCloudSavesUnavailable):
				notify("Cloud saves", fmt.Sprintf("No cloud saves found for %s.", game.Title))
			case err != nil:
				showErrorDialog(win, "Cannot back up saves", err)
			default:
				notify("Cloud saves", fmt.Sprintf("Backed up %d save %s for %s.",
					len(result.Files), pluralize(len(result.Files), "file", "files"), game.Title))
			}
		})
	}()
}

func pluralize(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
