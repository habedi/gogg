package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

func RefreshCatalogueAction(win fyne.Window, authService *auth.Service, onFinish func()) {
	progress := widget.NewProgressBar()
	ctx, cancel := context.WithCancel(context.Background())
	content := container.NewVBox(widget.NewLabel("Downloading latest game data..."), progress, widget.NewButton("Cancel", cancel))
	dlg := dialog.NewCustom("Refreshing Catalogue", "Dismiss", content, win)
	dlg.Show()

	go func() {
		defer onFinish()
		defer dlg.Hide()

		token, err := authService.RefreshToken()
		if err != nil || token == nil {
			showErrorDialog(win, "Failed to refresh token. Did you login?", err)
			return
		}

		gameIDs, err := client.FetchIdOfOwnedGames(token.AccessToken, "https://embed.gog.com/user/data/games")
		if err != nil {
			showErrorDialog(win, "Failed to fetch list of owned games", err)
			return
		}

		if len(gameIDs) == 0 {
			dialog.ShowInformation("Info", "No games found in your GOG account.", win)
			return
		}
		if err := db.EmptyCatalogue(); err != nil {
			showErrorDialog(win, "Failed to clear existing catalogue", err)
			return
		}

		total := float64(len(gameIDs))
		progress.Max = total
		var wg sync.WaitGroup
		taskChan := make(chan int, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for id := range taskChan {
					select {
					case <-ctx.Done():
						return
					default:
						url := fmt.Sprintf("https://embed.gog.com/account/gameDetails/%d.json", id)
						details, raw, fetchErr := client.FetchGameData(token.AccessToken, url)
						if fetchErr == nil && details.Title != "" {
							_ = db.PutInGame(id, details.Title, raw)
						}
						runOnMain(func() { progress.SetValue(progress.Value + 1) })
					}
				}
			}()
		}

		for _, id := range gameIDs {
			select {
			case <-ctx.Done():
				break
			case taskChan <- id:
			}
		}
		close(taskChan)
		wg.Wait()

		if ctx.Err() == nil {
			dialog.ShowInformation("Success", fmt.Sprintf("Successfully refreshed catalogue with %d games.", len(gameIDs)), win)
		}
	}()
}

func ExportCatalogueAction(win fyne.Window, format string) {
	defaultName := fmt.Sprintf("gogg_catalogue.%s", format)
	fileDialog := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil {
			showErrorDialog(win, "File save error", err)
			return
		}
		if uc == nil {
			return
		}
		defer uc.Close()

		games, err := db.GetCatalogue()
		if err != nil {
			showErrorDialog(win, "Failed to read catalogue from database", err)
			return
		}
		if len(games) == 0 {
			dialog.ShowInformation("Info", "Catalogue is empty. Nothing to export.", win)
			return
		}

		var exportErr error
		if format == "json" {
			enc := json.NewEncoder(uc)
			enc.SetIndent("", "  ")
			exportErr = enc.Encode(games)
		} else { // csv
			if _, err := fmt.Fprintln(uc, "ID,Title"); err != nil {
				exportErr = err
			} else {
				for _, g := range games {
					title := strings.ReplaceAll(g.Title, "\"", "\"\"")
					if _, err := fmt.Fprintf(uc, "%d,\"%s\"\n", g.ID, title); err != nil {
						exportErr = err
						break
					}
				}
			}
		}

		if exportErr != nil {
			showErrorDialog(win, "Failed to write export file", exportErr)
		} else {
			dialog.ShowInformation("Success", "Catalogue exported successfully.", win)
		}
	}, win)
	fileDialog.SetFileName(defaultName)
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{"." + format}))
	fileDialog.Resize(fyne.NewSize(800, 600))
	fileDialog.Show()
}

func showErrorDialog(win fyne.Window, msg string, err error) {
	detail := msg
	if err != nil {
		detail = fmt.Sprintf("%s\nError: %v", msg, err)
	}
	d := dialog.NewError(errors.New(detail), win)
	d.Show()
}
