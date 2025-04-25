// ui/catalogue.go
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

// CopyableLabel definition remains the same...
type CopyableLabel struct {
	widget.Label
	win fyne.Window
}

func NewCopyableLabel(win fyne.Window) *CopyableLabel {
	cl := &CopyableLabel{win: win}
	cl.ExtendBaseWidget(cl)
	return cl
}

func (cl *CopyableLabel) Tapped(_ *fyne.PointEvent) {
	cl.win.Clipboard().SetContent(cl.Text)
}

func (cl *CopyableLabel) TappedSecondary(_ *fyne.PointEvent) {}

// CatalogueListUI definition remains the same...
func CatalogueListUI(win fyne.Window) fyne.CanvasObject {
	var games []db.Game
	var data [][]string // Declare data slice here to be accessible in callbacks
	var table *widget.Table

	// Function to populate the data slice from the games list
	populateData := func() {
		data = [][]string{
			{"Row ID", "Game ID", "Game Title"}, // Header row
		}
		for i, game := range games {
			cleanedTitle := strings.ReplaceAll(game.Title, "\n", " ")
			data = append(data, []string{
				strconv.Itoa(i + 1),
				strconv.Itoa(game.ID),
				cleanedTitle,
			})
		}
	}

	// Initial fetch and data population
	var initialErr error
	games, initialErr = db.GetCatalogue()
	if initialErr != nil {
		// Show error but still return a placeholder label
		dialog.ShowError(fmt.Errorf("initial catalogue load failed: %v", initialErr), win)
		return widget.NewLabel("Error loading catalogue. Try refreshing.")
	}
	if len(games) == 0 {
		// If initially empty, data will just contain the header
		populateData()
		// We'll still create the table structure below, but it will show "empty"
	} else {
		populateData()
	}

	// Create a table widget using CopyableLabel for each cell.
	// The data functions now reference the 'data' slice in the outer scope.
	table = widget.NewTable(
		func() (int, int) {
			// Check if data is initialized and has at least one row (header)
			if len(data) == 0 || len(data[0]) == 0 {
				return 0, 0 // Return 0 rows/cols if data is empty or malformed
			}
			return len(data), len(data[0])
		},
		func() fyne.CanvasObject {
			return NewCopyableLabel(win)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			// Add bounds checking for safety
			if id.Row >= 0 && id.Row < len(data) && id.Col >= 0 && id.Col < len(data[id.Row]) {
				cell.(*CopyableLabel).SetText(data[id.Row][id.Col])
			} else {
				cell.(*CopyableLabel).SetText("") // Set empty text if out of bounds
			}
		},
	)
	// Manually set column widths (Fyne’s Table doesn’t auto-size columns).
	table.SetColumnWidth(0, 60)
	table.SetColumnWidth(1, 130)
	table.SetColumnWidth(2, 400)

	scroll := container.NewScroll(table)

	copyButton := widget.NewButton("Copy All", func() {
		// This now uses the potentially updated 'data' slice
		text := formatTableData(data)
		win.Clipboard().SetContent(text)
		dialog.ShowInformation("Copied", "Table data copied successfully.", win)
	})

	refreshButton := widget.NewButton("Refresh", func() {
		// Re-fetch games from the database
		newGames, err := db.GetCatalogue()
		if err != nil {
			dialog.ShowError(fmt.Errorf("failed to refresh catalogue: %v", err), win)
			return
		}
		// Update the games list and repopulate the data slice
		games = newGames
		populateData() // This updates the 'data' slice

		// Refresh the table widget to reflect the changes
		table.Refresh()
		dialog.ShowInformation("Refreshed", "Game list updated.", win)

	})

	// Put the buttons at the top, table in the center so it expands.
	topButtons := container.NewHBox(copyButton, refreshButton)
	return container.NewBorder(
		topButtons, // top
		nil,        // bottom
		nil,        // left
		nil,        // right
		scroll,     // center
	)
}

// SearchCatalogueUI shows a table of search results plus "Copy All" and "Clear" buttons.
// *** MODIFIED SIGNATURE to accept onClear callback ***
func SearchCatalogueUI(win fyne.Window, query string, searchByID bool, onClear func()) fyne.CanvasObject {
	var games []db.Game
	var err error

	// NOTE: Query validation happens in the caller (CatalogueTabUI)

	if searchByID {
		gameID, convErr := strconv.Atoi(query)
		if convErr != nil {
			// This case should ideally be caught earlier, but handle defensively
			return widget.NewLabel("Error: Invalid game ID. It must be a number.")
		}
		game, dbErr := db.GetGameByID(gameID)
		err = dbErr // Assign potential DB error
		if err == nil && game != nil {
			games = append(games, *game)
		}
	} else {
		games, err = db.SearchGamesByName(query)
	}

	// Handle database errors after the search attempt
	if err != nil {
		return widget.NewLabel(fmt.Sprintf("Database Error: %v", err))
	}

	// Handle no results found
	if len(games) == 0 {
		return widget.NewLabel("No game(s) found matching the query.")
	}

	// --- Build results table (same as before) ---
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
	table.SetColumnWidth(0, 60)
	table.SetColumnWidth(1, 130)
	table.SetColumnWidth(2, 400)

	scroll := container.NewScroll(table)

	// --- Add buttons ---
	copyButton := widget.NewButton("Copy All", func() {
		text := formatTableData(data)
		win.Clipboard().SetContent(text)
		dialog.ShowInformation("Copied", "Table data copied successfully.", win)
	})

	// *** ADDED Clear Button - Calls the passed-in onClear callback ***
	clearButton := widget.NewButton("Clear", onClear)

	// Place buttons together at the top
	topButtons := container.NewHBox(copyButton, clearButton) // Add clearButton here

	return container.NewBorder(
		topButtons, // top
		nil, nil, nil,
		scroll, // center (results table)
	)
}

// formatTableData definition remains the same...
func formatTableData(data [][]string) string {
	var sb strings.Builder
	for _, row := range data {
		sb.WriteString(strings.Join(row, "\t"))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// RefreshCatalogueUI definition remains the same...
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
						// Check if progress bar is still valid before updating
						if progress != nil {
							progress.SetValue(progress.Value + (1.0 / total))
						}
						mu.Unlock()
					}
				}
			}()
		}
		// Feed tasks into taskChan, checking for cancellation.
		for _, gameID := range games {
			select {
			case <-ctx.Done():
				break // Stop feeding tasks if cancelled
			case taskChan <- gameID:
			}
		}
		close(taskChan) // Close channel once all tasks are sent or cancellation occurred
		wg.Wait()       // Wait for workers to finish processing remaining tasks or exit due to context cancellation
		// Check if progress bar is still valid before setting final value
		if progress != nil {
			progress.SetValue(1)
		}
		dlg.Hide()
		if ctx.Err() == context.Canceled {
			dialog.ShowInformation("Cancelled", "Catalogue refresh was cancelled.", win)
		} else {
			dialog.ShowInformation("Success", "Refreshed the game catalogue successfully.", win)
		}
	}()
}

// ExportCatalogueUI definition remains the same...
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
			return // User cancelled
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

// exportCatalogueToWriter definition remains the same...
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

// exportCatalogueToCSV definition remains the same...
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
		// Properly escape double quotes in the title by replacing " with ""
		escapedTitle := strings.ReplaceAll(game.Title, "\"", "\"\"")
		if _, err := fmt.Fprintf(w, "%d,\"%s\"\n", game.ID, escapedTitle); err != nil {
			return err
		}
	}
	return nil
}

// exportCatalogueToJSON definition remains the same...
func exportCatalogueToJSON(w io.Writer) error {
	games, err := db.GetCatalogue()
	if err != nil {
		return err
	}
	if len(games) == 0 {
		return fmt.Errorf("no games found to export; did you refresh the catalogue")
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ") // Pretty print the JSON
	return encoder.Encode(games)
}
