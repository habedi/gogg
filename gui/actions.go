package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	statusLabel := widget.NewLabel("Preparing to refresh...")
	ctx, cancel := context.WithCancel(context.Background())
	content := container.NewVBox(statusLabel, progress, widget.NewButton("Cancel", cancel))
	dlg := dialog.NewCustom("Refreshing Catalogue", "Hide", content, win)
	dlg.Resize(fyne.NewSize(400, 200))
	dlg.Show()

	go func() {
		progressCb := func(p float64) {
			runOnMain(func() {
				statusLabel.SetText(fmt.Sprintf("Processing games... (%.0f%%)", p*100))
				progress.SetValue(p)
			})
		}

		err := client.RefreshCatalogue(ctx, authService, 10, progressCb)

		runOnMain(func() {
			// 1. First, always hide the progress dialog.
			dlg.Hide()

			// 2. Then, show the appropriate result dialog.
			if errors.Is(err, context.Canceled) {
				games, dbErr := db.GetCatalogue()
				var msg string
				if dbErr != nil {
					msg = "Refresh was cancelled. Could not retrieve partial game count."
				} else {
					msg = fmt.Sprintf("Refresh was cancelled.\n%d games were loaded before stopping.", len(games))
				}
				dialog.ShowInformation("Refresh Cancelled", msg, win)
			} else if err != nil {
				showErrorDialog(win, "Failed to refresh catalogue", err)
			} else { // Success
				games, dbErr := db.GetCatalogue()
				if dbErr != nil {
					dialog.ShowInformation("Success", "Successfully refreshed catalogue.", win)
				} else {
					successMsg := fmt.Sprintf("Successfully refreshed catalogue.\nYour library now contains %d games.", len(games))
					dialog.ShowInformation("Success", successMsg, win)
				}
			}

			// 3. Finally, run the onFinish callback to update the UI (re-enable button, etc).
			onFinish()
		})
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
