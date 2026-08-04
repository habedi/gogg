package gui

import (
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
)

// gameSelection tracks which games are ticked in the library list. Selection is
// keyed by game so it survives searching, sorting and filtering. It is only
// touched from the UI thread.
type gameSelection struct {
	ids map[int]bool
}

func newGameSelection() *gameSelection {
	return &gameSelection{ids: make(map[int]bool)}
}

func (s *gameSelection) has(id int) bool { return s.ids[id] }

func (s *gameSelection) set(id int, selected bool) {
	if selected {
		s.ids[id] = true
		return
	}
	delete(s.ids, id)
}

func (s *gameSelection) count() int { return len(s.ids) }

func (s *gameSelection) clear() { s.ids = make(map[int]bool) }

// selectAll ticks every game given, leaving anything not in the list alone.
func (s *gameSelection) selectAll(games []db.Game) {
	for _, game := range games {
		s.ids[game.ID] = true
	}
}

// gamesIn returns the selected games among those given, in the order given.
// Games that have left the catalogue simply drop out.
func (s *gameSelection) gamesIn(games []db.Game) []db.Game {
	selected := make([]db.Game, 0, len(s.ids))
	for _, game := range games {
		if s.has(game.ID) {
			selected = append(selected, game)
		}
	}
	return selected
}

// bindCheck points a recycled row's checkbox at a new item. The old handler is
// detached first, because SetChecked fires OnChanged and would otherwise write
// the new item's state onto the item the row showed before.
func bindCheck(check *widget.Check, checked bool, onChanged func(bool)) {
	check.OnChanged = nil
	check.SetChecked(checked)
	check.OnChanged = onChanged
}

// gameRow is a row in the library list. It is a widget rather than a bare
// container so the parts are reached by name instead of by index, and so the
// title can take the width left over by the leading controls.
type gameRow struct {
	widget.BaseWidget
	check      *widget.Check
	downloaded *widget.Icon
	updateBtn  *widget.Button
	title      *widget.Label
}

// newGameRow builds an empty row for the library list.
func newGameRow() fyne.CanvasObject {
	row := &gameRow{
		check:      widget.NewCheck("", nil),
		downloaded: widget.NewIcon(theme.ConfirmIcon()),
		updateBtn:  widget.NewButtonWithIcon("", theme.DownloadIcon(), nil),
		title:      widget.NewLabel("Game Title"),
	}
	row.downloaded.Hide()
	row.updateBtn.Hide()
	row.updateBtn.Importance = widget.LowImportance
	row.title.Truncation = fyne.TextTruncateEllipsis

	row.ExtendBaseWidget(row)
	return row
}

func (r *gameRow) CreateRenderer() fyne.WidgetRenderer {
	leading := container.NewHBox(r.check, r.downloaded, r.updateBtn)
	// The title is the centre of a border layout, so it is given whatever width
	// the leading controls leave and ellipsises only when it truly runs out.
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, leading, nil, r.title))
}

// bindGameRow fills a recycled row with a game. onToggle runs when the row's
// checkbox is changed by the user.
func bindGameRow(row fyne.CanvasObject, game db.Game, sel *gameSelection, onToggle func()) {
	r, ok := row.(*gameRow)
	if !ok {
		return
	}

	bindCheck(r.check, sel.has(game.ID), func(checked bool) {
		sel.set(game.ID, checked)
		if onToggle != nil {
			onToggle()
		}
	})

	r.title.SetText(game.Title)

	if !isGameDownloadedCached(game.ID) {
		r.downloaded.Hide()
		r.updateBtn.Hide()
		return
	}

	r.downloaded.Show()
	hasUpdate, diff := hasGameUpdateCached(game.ID)
	if !hasUpdate {
		r.updateBtn.Hide()
		r.updateBtn.SetText("")
		return
	}

	r.updateBtn.Show()
	r.updateBtn.SetText(fmt.Sprintf("%d", len(diff)))
	r.updateBtn.OnTapped = func() {
		content := container.NewVBox()
		for _, line := range diff {
			content.Add(widget.NewLabel(line))
		}
		dialog.ShowCustom("Update details", "Close", container.NewVScroll(content),
			fyne.CurrentApp().Driver().AllWindows()[0])
	}
}

// batchResult summarises what a batch of downloads did.
type batchResult struct {
	Queued  int
	Skipped []string // titles already downloading or waiting in the queue
	Failed  []string // titles that could not be started at all
}

// queueDownloads queues every game, carrying on past the ones that cannot be
// queued so a single bad game does not stop the batch.
func queueDownloads(dm *DownloadManager, games []db.Game, build func(db.Game) queuedDownload) batchResult {
	var result batchResult
	for _, game := range games {
		err := dm.QueueOrStart(build(game))
		switch {
		case err == nil:
			result.Queued++
		case errors.Is(err, ErrDownloadInProgress):
			result.Skipped = append(result.Skipped, game.Title)
		default:
			result.Failed = append(result.Failed, game.Title)
			log.Error().Err(err).Str("game", game.Title).Msg("Failed to queue download")
		}
	}
	return result
}

func (r batchResult) summary() string {
	parts := []string{fmt.Sprintf("Queued %d %s for download.", r.Queued, gamesWord(r.Queued))}
	if len(r.Skipped) > 0 {
		parts = append(parts, fmt.Sprintf("%d %s already in progress: %s",
			len(r.Skipped), gamesWord(len(r.Skipped)), joinTitles(r.Skipped)))
	}
	if len(r.Failed) > 0 {
		parts = append(parts, fmt.Sprintf("%d %s could not be started: %s",
			len(r.Failed), gamesWord(len(r.Failed)), joinTitles(r.Failed)))
	}
	return strings.Join(parts, "\n")
}

func gamesWord(n int) string {
	if n == 1 {
		return "game"
	}
	return "games"
}

// joinTitles lists titles, keeping the message readable for large batches.
func joinTitles(titles []string) string {
	const maxListed = 10
	if len(titles) <= maxListed {
		return strings.Join(titles, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(titles[:maxListed], ", "), len(titles)-maxListed)
}
