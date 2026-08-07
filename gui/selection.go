package gui

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
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

// statusBadges are the marks a game carries wherever it is listed: whether it
// has been downloaded, how many files an update would change, and how far along
// a download running right now is. The list and the grid show the same ones, so
// they are built and filled in one place.
type statusBadges struct {
	downloaded *widget.Icon
	update     *iconButton
	// progress is the download happening now, shown where the game is listed
	// rather than only in the Downloads tab.
	progress *progressBadge
}

// progressBadge is a progress bar that asks only for the width a list row can
// spare. Containers that have more to give, like a grid cell, still stretch it.
type progressBadge struct {
	widget.ProgressBar
}

// progressBadgeWidth is the least width the bar asks for in a list row.
const progressBadgeWidth float32 = 96

func newProgressBadge() *progressBadge {
	bar := &progressBadge{}
	// The bar is the message; a percentage in every row is noise.
	bar.TextFormatter = func() string { return "" }
	bar.ExtendBaseWidget(bar)
	return bar
}

func (p *progressBadge) MinSize() fyne.Size {
	size := p.ProgressBar.MinSize()
	size.Width = progressBadgeWidth
	return size
}

func newStatusBadges() *statusBadges {
	badges := &statusBadges{
		downloaded: widget.NewIcon(theme.NewSuccessThemedResource(theme.ConfirmIcon())),
		update:     newIconButton(theme.NewWarningThemedResource(theme.DownloadIcon()), "What this update changes", nil),
		progress:   newProgressBadge(),
	}
	badges.downloaded.Hide()
	badges.update.Hide()
	badges.progress.Hide()
	return badges
}

// show marks a game with what is known about it. Tapping the update badge lists
// what has changed. dm may be nil, in which case no download can be running.
func (b *statusBadges) show(gameID int, dm *DownloadManager) {
	if task := dm.runningTaskFor(gameID); task != nil {
		// While its files are on their way, the game is its progress: the other
		// marks describe a state it is about to leave.
		b.downloaded.Hide()
		b.update.Hide()
		b.progress.Bind(task.Progress)
		b.progress.Show()
		return
	}
	b.progress.Unbind()
	b.progress.Hide()

	if !isGameDownloadedCached(gameID) {
		b.downloaded.Hide()
		b.update.Hide()
		return
	}

	b.downloaded.Show()
	hasUpdate, diff := hasGameUpdateCached(gameID)
	if !hasUpdate {
		b.update.Hide()
		b.update.SetText("")
		return
	}

	b.update.Show()
	b.update.SetText(fmt.Sprintf("%d", len(diff)))
	b.update.OnTapped = func() {
		dialog.ShowCustom("Update details", "Close", updateDetailsBody(diff),
			fyne.CurrentApp().Driver().AllWindows()[0])
	}
}

// gameRow is a row in the library list. It is a widget rather than a bare
// container so the parts are reached by name instead of by index, and so the
// title can take the width left over by the leading controls.
type gameRow struct {
	widget.BaseWidget
	gameID    int
	check     *widget.Check
	thumbnail *canvas.Image
	badges    *statusBadges
	title     *widget.Label
}

// thumbnailSize is the artwork a row shows: small enough to keep rows compact.
var thumbnailSize = fyne.NewSize(64, 36)

// newGameRow builds an empty row for the library list.
func newGameRow() fyne.CanvasObject {
	row := &gameRow{
		check:     widget.NewCheck("", nil),
		thumbnail: canvas.NewImageFromResource(theme.FileImageIcon()),
		badges:    newStatusBadges(),
		title:     widget.NewLabel("Game Title"),
	}
	row.thumbnail.FillMode = canvas.ImageFillContain
	row.thumbnail.SetMinSize(thumbnailSize)
	row.title.Truncation = fyne.TextTruncateEllipsis

	row.ExtendBaseWidget(row)
	return row
}

func (r *gameRow) CreateRenderer() fyne.WidgetRenderer {
	leading := container.NewHBox(r.check, r.thumbnail, r.badges.downloaded, r.badges.update)
	// The title is the centre of a border layout, so it is given whatever width
	// the leading controls leave and ellipsises only when it truly runs out.
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, leading, r.badges.progress, r.title))
}

// bindGameRow fills a recycled row with a game. dm says whether a download is
// running for it, and onToggle runs when the row's checkbox is changed by the
// user.
func bindGameRow(row fyne.CanvasObject, game db.Game, sel *gameSelection, covers *coverCache,
	dm *DownloadManager, onToggle func(),
) {
	r, ok := row.(*gameRow)
	if !ok {
		return
	}

	// Rows are rebound on every refresh; only a row pointed at a different game
	// needs its thumbnail replaced.
	sameGame := r.gameID == game.ID
	r.gameID = game.ID

	bindCheck(r.check, sel.has(game.ID), func(checked bool) {
		sel.set(game.ID, checked)
		if onToggle != nil {
			onToggle()
		}
	})

	r.title.SetText(game.Title)
	r.loadThumbnail(game, covers, sameGame)
	r.badges.show(game.ID, dm)
}

// How much room the list of changes may take before it starts scrolling.
const (
	updateDetailsMaxWidth  float32 = 620
	updateDetailsMaxHeight float32 = 320
)

// updateDetailsBody lists what has changed, at a size that shows it. A scroll
// left to its own minimum opens one line tall whatever the list says, so the
// list is measured and the dialog given that much, up to a limit.
func updateDetailsBody(diff []string) *container.Scroll {
	changes := container.NewVBox()
	for _, line := range diff {
		changes.Add(widget.NewLabel(line))
	}

	body := container.NewVScroll(changes)
	wanted := changes.MinSize()
	body.SetMinSize(fyne.NewSize(
		min(wanted.Width, updateDetailsMaxWidth), min(wanted.Height, updateDetailsMaxHeight)))
	return body
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

// loadThumbnail fetches the row's artwork unless it is already on screen.
func (r *gameRow) loadThumbnail(game db.Game, covers *coverCache, sameGame bool) {
	if sameGame && r.thumbnail.Image != nil {
		return
	}

	r.thumbnail.Resource = theme.FileImageIcon()
	r.thumbnail.Image = nil
	r.thumbnail.Refresh()

	if covers == nil {
		return
	}
	covers.load(game, coverThumbnail, func(id int) bool { return r.gameID == id },
		func(data []byte, source coverSource) {
			decoded, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return
			}
			if source.Faded {
				decoded = cropArtwork(decoded)
			}
			r.thumbnail.Resource = nil
			r.thumbnail.Image = decoded
			r.thumbnail.Refresh()
		})
}
