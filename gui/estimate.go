package gui

import (
	"encoding/csv"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
)

// gameSizeEstimate is how much disk one game would take.
type gameSizeEstimate struct {
	Title string
	Bytes int64
}

// estimateSelection sizes each game with the settings currently on the download
// form, so the answer matches what downloading them would actually fetch.
// Games whose stored data cannot be read are listed as unknown rather than
// dropped, so the list still accounts for everything that was asked about.
func estimateSelection(s *libraryState, games []db.Game) (estimates []gameSizeEstimate, total int64) {
	estimates = make([]gameSizeEstimate, 0, len(games))
	for _, game := range games {
		size := s.estimateSize(game)
		estimates = append(estimates, gameSizeEstimate{Title: game.Title, Bytes: size})
		total += size
	}
	return estimates, total
}

// sizeEstimateCSV renders an estimate for the clipboard.
func sizeEstimateCSV(estimates []gameSizeEstimate, total int64) string {
	var out strings.Builder
	writer := csv.NewWriter(&out)
	_ = writer.Write([]string{"Game", "Size", "Bytes"})
	for _, estimate := range estimates {
		_ = writer.Write([]string{
			estimate.Title, formatBytes(estimate.Bytes), fmt.Sprintf("%d", estimate.Bytes),
		})
	}
	_ = writer.Write([]string{"Total", formatBytes(total), fmt.Sprintf("%d", total)})
	writer.Flush()
	return out.String()
}

// showSizeEstimate presents an estimate with a way to take it away.
func showSizeEstimate(win fyne.Window, estimates []gameSizeEstimate, total int64) {
	list := widget.NewList(
		func() int { return len(estimates) },
		func() fyne.CanvasObject {
			title := widget.NewLabel("Game")
			title.Truncation = fyne.TextTruncateEllipsis
			return container.NewBorder(nil, nil, nil, widget.NewLabel("0 B"), title)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(estimates) {
				return
			}
			row := obj.(*fyne.Container)
			row.Objects[0].(*widget.Label).SetText(estimates[id].Title)
			row.Objects[1].(*widget.Label).SetText(formatBytes(estimates[id].Bytes))
		},
	)

	summary := widget.NewLabel(fmt.Sprintf("%d %s · %s total",
		len(estimates), gamesWord(len(estimates)), formatBytes(total)))
	summary.TextStyle = fyne.TextStyle{Bold: true}

	var copyBtn *widget.Button
	copyBtn = widget.NewButtonWithIcon("Copy as CSV", theme.ContentCopyIcon(), func() {
		fyne.CurrentApp().Clipboard().SetContent(sizeEstimateCSV(estimates, total))
		showCopied(copyBtn, "Copied")
	})

	content := container.NewBorder(summary, copyBtn, nil, nil, list)
	estimate := dialog.NewCustom("Storage Size", "Close", content, win)
	estimate.Resize(fyne.NewSize(660, 520))
	estimate.Show()
}
