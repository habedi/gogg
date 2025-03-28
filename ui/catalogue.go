package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// CopyableLabel is a label that copies its text to the clipboard when tapped.
type CopyableLabel struct {
	widget.Label
	win fyne.Window
}

// NewCopyableLabel creates a new CopyableLabel.
func NewCopyableLabel(win fyne.Window) *CopyableLabel {
	cl := &CopyableLabel{win: win}
	cl.ExtendBaseWidget(cl)
	return cl
}

// Tapped is called when the label is tapped.
func (cl *CopyableLabel) Tapped(_ *fyne.PointEvent) {
	cl.win.Clipboard().SetContent(cl.Text)
	// Optionally, you could show a brief notification here.
}

// TappedSecondary is required to implement Tappable but does nothing.
func (cl *CopyableLabel) TappedSecondary(_ *fyne.PointEvent) {}

// CatalogueListUI shows a table of all games plus a "Copy All" button at the top.
func CatalogueListUI(win fyne.Window) fyne.CanvasObject {
	games, err := db.GetCatalogue()
	if err != nil {
		dialog.ShowError(fmt.Errorf("unable to list games: %v", err), win)
		return widget.NewLabel("Error loading catalogue")
	}
	if len(games) == 0 {
		return widget.NewLabel("Game catalogue is empty. Did you refresh the catalogue?")
	}

	// Prepare table data with headers.
	data := [][]string{
		{"Row ID", "Game ID", "Game Title"},
	}
	for i, game := range games {
		cleanedTitle := strings.ReplaceAll(game.Title, "\n", " ")
		data = append(data, []string{
			strconv.Itoa(i + 1),
			strconv.Itoa(game.ID),
			cleanedTitle,
		})
	}

	// Create a table widget using CopyableLabel for each cell.
	table := widget.NewTable(
		func() (int, int) {
			return len(data), len(data[0])
		},
		func() fyne.CanvasObject {
			return NewCopyableLabel(win)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			cell.(*CopyableLabel).SetText(data[id.Row][id.Col])
		},
	)
	// Manually set column widths (Fyne’s Table doesn’t auto-size columns).
	table.SetColumnWidth(0, 60)
	table.SetColumnWidth(1, 130)
	table.SetColumnWidth(2, 400)

	scroll := container.NewScroll(table)

	copyButton := widget.NewButton("Copy All", func() {
		text := formatTableData(data)
		win.Clipboard().SetContent(text)
		dialog.ShowInformation("Copied", "Table data copied successfully.", win)
	})

	// Put the copy button at the top, table in the center so it expands.
	return container.NewBorder(
		container.NewHBox(copyButton), // top
		nil,                           // bottom
		nil,                           // left
		nil,                           // right
		scroll,                        // center
	)
}

// SearchCatalogueUI shows a table of search results plus a "Copy All" button at the top.
func SearchCatalogueUI(win fyne.Window, query string, searchByID bool) fyne.CanvasObject {
	var games []db.Game
	var err error

	// Check if the query is empty.
	if query == "" {
		return widget.NewLabel("Please enter a game title or ID to search.")
	}

	if searchByID {
		gameID, convErr := strconv.Atoi(query)
		if convErr != nil {
			return widget.NewLabel("Error: Invalid game ID. It must be a number.")
		}
		game, err := db.GetGameByID(gameID)
		if err != nil {
			return widget.NewLabel(fmt.Sprintf("Error: %v", err))
		}
		if game != nil {
			games = append(games, *game)
		}
	} else {
		games, err = db.SearchGamesByName(query)
		if err != nil {
			return widget.NewLabel(fmt.Sprintf("Error: %v", err))
		}
	}

	if len(games) == 0 {
		return widget.NewLabel("No game(s) found matching the query.")
	}

	data := [][]string{
		{"Row ID", "Game ID", "Title"},
	}
	for i, game := range games {
		data = append(data, []string{
			strconv.Itoa(i + 1),
			strconv.Itoa(game.ID),
			game.Title,
		})
	}

	table := widget.NewTable(
		func() (int, int) { return len(data), len(data[0]) },
		func() fyne.CanvasObject { return NewCopyableLabel(win) },
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			cell.(*CopyableLabel).SetText(data[id.Row][id.Col])
		},
	)
	// Manually set column widths.
	table.SetColumnWidth(0, 60)
	table.SetColumnWidth(1, 130)
	table.SetColumnWidth(2, 400)

	scroll := container.NewScroll(table)

	copyButton := widget.NewButton("Copy All", func() {
		text := formatTableData(data)
		win.Clipboard().SetContent(text)
		dialog.ShowInformation("Copied", "Table data copied successfully.", win)
	})

	return container.NewBorder(
		container.NewHBox(copyButton), // top
		nil, nil, nil,
		scroll,
	)
}

// formatTableData converts a 2D string slice into a tab-separated string for copying.
func formatTableData(data [][]string) string {
	var sb strings.Builder
	for _, row := range data {
		sb.WriteString(strings.Join(row, "\t"))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// RefreshCatalogueUI refreshes the game catalogue while showing a progress dialog.
// It now supports cancellation via the cancel button.
func RefreshCatalogueUI(win fyne.Window) {
	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 1
	progress.SetValue(0)

	// Create a cancellable context.
	ctx, cancel := context.WithCancel(context.Background())

	// Build a custom dialog content with a cancel button.
	cancelButton := widget.NewButton("Cancel", func() {
		cancel()
	})
	content := container.NewVBox(
		widget.NewLabel("Refreshing game catalogue..."),
		progress,
		cancelButton,
	)
	dlg := dialog.NewCustom("Refreshing Catalogue", "", content, win)
	dlg.Show()

	go func() {
		numThreads := 10

		token, err := client.RefreshToken()
		if err != nil || token == nil {
			dlg.Hide()
			if errors.Is(ctx.Err(), context.Canceled) {
				dialog.ShowInformation("Cancelled", "Catalogue refresh was cancelled.", win)
			} else {
				dialog.ShowError(fmt.Errorf("failed to refresh token; did you login"), win)
			}
			return
		}
		games, err := client.FetchIdOfOwnedGames(token.AccessToken, "https://embed.gog.com/user/data/games")
		if err != nil {
			dlg.Hide()
			if errors.Is(ctx.Err(), context.Canceled) {
				dialog.ShowInformation("Cancelled", "Catalogue refresh was cancelled.", win)
			} else {
				dialog.ShowError(fmt.Errorf("error fetching games: %v", err), win)
			}
			return
		}
		if len(games) == 0 {
			dlg.Hide()
			dialog.ShowInformation("Info", "No games found in the GOG account.", win)
			return
		}
		if err := db.EmptyCatalogue(); err != nil {
			dlg.Hide()
			if errors.Is(ctx.Err(), context.Canceled) {
				dialog.ShowInformation("Cancelled", "Catalogue refresh was cancelled.", win)
			} else {
				dialog.ShowError(fmt.Errorf("failed to empty catalogue: %v", err), win)
			}
			return
		}

		total := float64(len(games))
		var mu sync.Mutex
		var wg sync.WaitGroup
		taskChan := make(chan int, 10)

		// Spawn worker goroutines that check for cancellation.
		for i := 0; i < numThreads; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					case gameID, ok := <-taskChan:
						if !ok {
							return
						}
						url := fmt.Sprintf("https://embed.gog.com/account/gameDetails/%d.json", gameID)
						details, rawDetails, err := client.FetchGameData(token.AccessToken, url)
						if err == nil && details.Title != "" {
							_ = db.PutInGame(gameID, details.Title, rawDetails)
						}
						mu.Lock()
						progress.SetValue(progress.Value + 1/total)
						mu.Unlock()
					}
				}
			}()
		}
		// Feed tasks into taskChan, checking for cancellation.
		for _, gameID := range games {
			select {
			case <-ctx.Done():
				break
			case taskChan <- gameID:
			}
		}
		close(taskChan)
		wg.Wait()
		progress.SetValue(1)
		dlg.Hide()
		if ctx.Err() == context.Canceled {
			dialog.ShowInformation("Cancelled", "Catalogue refresh was cancelled.", win)
		} else {
			dialog.ShowInformation("Success", "Refreshed the game catalogue successfully.", win)
		}
	}()
}

// ExportCatalogueUI exports the catalogue using a file save dialog.
func ExportCatalogueUI(win fyne.Window, exportFormat string) {
	var defaultFileName string
	switch exportFormat {
	case "json":
		defaultFileName = "game_catalogue.json"
	case "csv":
		defaultFileName = "game_list.csv"
	default:
		dialog.ShowError(fmt.Errorf("unsupported export format: %s", exportFormat), win)
		return
	}

	fileDialog := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		if uc == nil {
			return
		}
		defer uc.Close()

		exportErr := exportCatalogueToWriter(uc, exportFormat)
		if exportErr != nil {
			dialog.ShowError(exportErr, win)
		} else {
			dialog.ShowInformation("Success", "File exported successfully.", win)
		}
	}, win)

	fileDialog.SetFileName(defaultFileName)
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{"." + exportFormat}))
	fileDialog.Show()
}

func exportCatalogueToWriter(w io.Writer, exportFormat string) error {
	switch exportFormat {
	case "json":
		return exportCatalogueToJSON(w)
	case "csv":
		return exportCatalogueToCSV(w)
	default:
		return fmt.Errorf("unsupported export format: %s", exportFormat)
	}
}

func exportCatalogueToCSV(w io.Writer) error {
	games, err := db.GetCatalogue()
	if err != nil {
		return err
	}
	if len(games) == 0 {
		return fmt.Errorf("no games found to export; did you refresh the catalogue")
	}
	if _, err := fmt.Fprintln(w, "ID,Title"); err != nil {
		return err
	}
	for _, game := range games {
		if _, err := fmt.Fprintf(w, "%d,\"%s\"\n", game.ID, game.Title); err != nil {
			return err
		}
	}
	return nil
}

func exportCatalogueToJSON(w io.Writer) error {
	games, err := db.GetCatalogue()
	if err != nil {
		return err
	}
	if len(games) == 0 {
		return fmt.Errorf("no games found to export; did you refresh the catalogue")
	}
	return json.NewEncoder(w).Encode(games)
}
