package gui

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

func CatalogueListUI(win fyne.Window) fyne.CanvasObject {
	games, err := db.GetCatalogue()
	if err != nil {
		dialog.ShowError(fmt.Errorf("initial catalogue load failed: %v", err), win)
		return widget.NewLabel("Error loading catalogue. Try refresh the catalogue and try again")
	}
	data := [][]string{{"Row ID", "Game ID", "Game Title"}}
	for i, game := range games {
		title := strings.ReplaceAll(game.Title, "\n", " ")
		data = append(data, []string{strconv.Itoa(i + 1), strconv.Itoa(game.ID), title})
	}
	table := widget.NewTable(
		func() (int, int) {
			if len(data) == 0 || len(data[0]) == 0 {
				return 0, 0
			}
			return len(data), len(data[0])
		},
		func() fyne.CanvasObject {
			return NewCopyableLabel(win)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			if id.Row < len(data) && id.Col < len(data[id.Row]) {
				cell.(*CopyableLabel).SetText(data[id.Row][id.Col])
			} else {
				cell.(*CopyableLabel).SetText("")
			}
		},
	)
	table.SetColumnWidth(0, 60)
	table.SetColumnWidth(1, 130)
	table.SetColumnWidth(2, 400)
	scroll := container.NewScroll(table)
	copyButton := widget.NewButton("Copy All", func() {
		win.Clipboard().SetContent(formatTableData(data))
		dialog.ShowInformation("Copied", "Game list copied to clipboard", win)
	})
	refreshButton := widget.NewButton("Refresh", func() {
		newGames, err := db.GetCatalogue()
		if err != nil {
			dialog.ShowError(fmt.Errorf("failed to refresh catalogue: %v", err), win)
			return
		}
		data = [][]string{{"Row ID", "Game ID", "Game Title"}}
		for i, game := range newGames {
			title := strings.ReplaceAll(game.Title, "\n", " ")
			data = append(data, []string{strconv.Itoa(i + 1), strconv.Itoa(game.ID), title})
		}
		table.Refresh()
		dialog.ShowInformation("Refreshed", "Game list updated", win)
	})
	topButtons := container.NewHBox(copyButton, refreshButton)
	return container.NewBorder(topButtons, nil, nil, nil, scroll)
}

func SearchCatalogueUI(win fyne.Window, query string, searchByID bool, onClear func()) fyne.CanvasObject {
	if strings.TrimSpace(query) == "" {
		return widget.NewLabel("Error: Search term or game ID cannot be empty")
	}
	var games []db.Game
	var err error
	if searchByID {
		id, convErr := strconv.Atoi(query)
		if convErr == nil {
			var g *db.Game
			g, err = db.GetGameByID(id)
			if err == nil && g != nil {
				games = append(games, *g)
			}
		} else {
			return widget.NewLabel("Error: Invalid game ID. A game ID must be a number")
		}
	} else {
		games, err = db.SearchGamesByName(query)
	}
	if err != nil {
		return widget.NewLabel(fmt.Sprintf("Database Error: %v", err))
	}
	if len(games) == 0 {
		return widget.NewLabel("No results found matching the search term or game ID")
	}
	data := [][]string{{"Row ID", "Game ID", "Game Title"}}
	for i, game := range games {
		data = append(data, []string{strconv.Itoa(i + 1), strconv.Itoa(game.ID), game.Title})
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
	copyButton := widget.NewButton("Copy Results", func() {
		win.Clipboard().SetContent(formatTableData(data))
		dialog.ShowInformation("Copied", "Results copied to clipboard", win)
	})
	clearButton := widget.NewButton("Clear Results", onClear)
	topButtons := container.NewHBox(copyButton, clearButton)
	return container.NewBorder(topButtons, nil, nil, nil, scroll)
}

func formatTableData(data [][]string) string {
	var sb strings.Builder
	for _, row := range data {
		sb.WriteString(strings.Join(row, "\t"))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func RefreshCatalogueUI(win fyne.Window) {
	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 1
	progress.SetValue(0)
	ctx, cancel := context.WithCancel(context.Background())
	cancelButton := widget.NewButton("Cancel", func() { cancel() })
	content := container.NewVBox(widget.NewLabel("Downloading the latest game data"), progress, cancelButton)
	dlg := dialog.NewCustom("Refreshing Catalogue", "OK", content, win)
	dlg.Show()
	go func() {
		token, err := client.RefreshToken()
		if err != nil || token == nil {
			fyne.Do(func() {
				dlg.Hide()
				if errors.Is(ctx.Err(), context.Canceled) {
					dialog.ShowInformation("Cancelled", "Refresh the catalogue was cancelled", win)
				} else {
					dialog.ShowError(fmt.Errorf("failed to refresh token; did you login"), win)
				}
			})
			return
		}
		gameIDs, err := client.FetchIdOfOwnedGames(token.AccessToken, "https://embed.gog.com/user/data/games")
		if err != nil {
			fyne.Do(func() {
				dlg.Hide()
				if errors.Is(ctx.Err(), context.Canceled) {
					dialog.ShowInformation("Cancelled", "Catalogue refresh was cancelled", win)
				} else {
					dialog.ShowError(fmt.Errorf("error fetching games: %v", err), win)
				}
			})
			return
		}
		if len(gameIDs) == 0 {
			fyne.Do(func() {
				dlg.Hide()
				dialog.ShowInformation("Info", "No games found in the GOG account", win)
			})
			return
		}
		if err := db.EmptyCatalogue(); err != nil {
			fyne.Do(func() {
				dlg.Hide()
				if errors.Is(ctx.Err(), context.Canceled) {
					dialog.ShowInformation("Cancelled", "Catalogue refresh was cancelled", win)
				} else {
					dialog.ShowError(fmt.Errorf("failed to empty catalogue: %v", err), win)
				}
			})
			return
		}
		total := float64(len(gameIDs))
		var mu sync.Mutex
		var wg sync.WaitGroup
		taskChan := make(chan int, 10)
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					case id, ok := <-taskChan:
						if !ok {
							return
						}
						url := fmt.Sprintf("https://embed.gog.com/account/gameDetails/%d.json", id)
						details, raw, err := client.FetchGameData(token.AccessToken, url)
						if err == nil && details.Title != "" {
							_ = db.PutInGame(id, details.Title, raw)
						}
						mu.Lock()
						v := progress.Value + (1.0 / total)
						mu.Unlock()
						fyne.Do(func() { progress.SetValue(v) })
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
		fyne.Do(func() {
			progress.SetValue(1)
			dlg.Hide()
			if errors.Is(ctx.Err(), context.Canceled) {
				dialog.ShowInformation("Cancelled", "Catalogue refresh was cancelled", win)
			} else {
				dialog.ShowInformation("Success", "Rebuilt the catalogue with the latest data", win)
			}
		})
	}()
}

func ExportCatalogueUI(win fyne.Window, exportFormat string) {
	var defaultName string
	switch exportFormat {
	case "json":
		defaultName = "game_catalogue.json"
	case "csv":
		defaultName = "game_list.csv"
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
		if e := exportCatalogueToWriter(uc, exportFormat); e != nil {
			dialog.ShowError(e, win)
		} else {
			dialog.ShowInformation("Success", "File exported successfully", win)
		}
	}, win)
	fileDialog.SetFileName(defaultName)
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{"." + exportFormat}))
	fileDialog.Resize(fyne.NewSize(800, 600))
	fileDialog.Show()
}

func exportCatalogueToWriter(w io.Writer, format string) error {
	switch format {
	case "json":
		return exportCatalogueToJSON(w)
	case "csv":
		return exportCatalogueToCSV(w)
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}

func exportCatalogueToCSV(w io.Writer) error {
	games, err := db.GetCatalogue()
	if err != nil {
		return err
	}
	if len(games) == 0 {
		return errors.New("no games found to export; did you refresh the catalogue")
	}
	if _, err := fmt.Fprintln(w, "ID,Title"); err != nil {
		return err
	}
	for _, g := range games {
		title := strings.ReplaceAll(g.Title, "\"", "\"\"")
		if _, err := fmt.Fprintf(w, "%d,\"%s\"\n", g.ID, title); err != nil {
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
		return errors.New("no games found to export; did you refresh the catalogue")
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(games)
}
