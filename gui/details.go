package gui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// gameDetail is one labelled fact about a game.
type gameDetail struct {
	Label string
	Value string
}

// gameDetails describes a game from what is already stored locally, so the
// pane costs nothing beyond reading the catalogue entry.
func gameDetails(game db.Game, dm *DownloadManager, meta *client.GameMetadata) []gameDetail {
	details := []gameDetail{{Label: "Game ID", Value: strconv.Itoa(game.ID)}}

	version := game.Version
	if version == "" {
		version = "Unknown"
	}
	details = append(details, gameDetail{Label: "Version", Value: version})

	// What GOG's store says, when it has been looked up.
	if meta != nil {
		details = appendIf(details, "Developer", strings.Join(meta.Developers, ", "))
		details = appendIf(details, "Publisher", meta.Publisher)
		details = appendIf(details, "Released", meta.ReleaseDate)
		details = appendIf(details, "Genres", strings.Join(meta.Genres, ", "))
		details = appendIf(details, "Features", strings.Join(meta.Features, ", "))
		details = appendIf(details, "Voiceovers", strings.Join(meta.Voiceovers, ", "))
		details = appendIf(details, "Age rating", meta.AgeRating)
		if meta.InstalledMB > 0 {
			details = append(details, gameDetail{
				Label: "Installed size", Value: formatBytes(meta.InstalledMB * 1024 * 1024),
			})
		}
	}

	if parsed, err := client.ParseGameData(game.Data); err == nil {
		if languages := offeredLanguages(parsed); len(languages) > 0 {
			details = append(details, gameDetail{Label: "Languages", Value: strings.Join(languages, ", ")})
		}
		if platforms := offeredPlatforms(parsed); len(platforms) > 0 {
			details = append(details, gameDetail{Label: "Platforms", Value: strings.Join(platforms, ", ")})
		}
		if n := len(parsed.DLCs); n > 0 {
			details = append(details, gameDetail{Label: "DLCs", Value: strconv.Itoa(n)})
		}
		if n := len(parsed.Extras); n > 0 {
			details = append(details, gameDetail{Label: "Extras", Value: strconv.Itoa(n)})
		}
	}

	if size := estimateGameSize(game); size > 0 {
		details = append(details, gameDetail{Label: "Estimated size", Value: formatBytes(size)})
	}

	if dir, ok := getGameDownloadDirectory(dm, game); ok {
		details = append(details, gameDetail{Label: "Downloaded to", Value: dir})
	} else {
		details = append(details, gameDetail{Label: "Downloaded", Value: "No"})
	}

	if hasUpdate, diff := hasGameUpdateCached(game.ID); hasUpdate {
		details = append(details, gameDetail{
			Label: "Update",
			Value: fmt.Sprintf("%d changed %s", len(diff), filesWord(len(diff))),
		})
	}

	return details
}

// offeredLanguages lists the languages a game ships in.
func offeredLanguages(game client.Game) []string {
	seen := make(map[string]bool)
	languages := make([]string, 0, len(game.Downloads))
	for _, download := range game.Downloads {
		if download.Language == "" || seen[download.Language] {
			continue
		}
		seen[download.Language] = true
		languages = append(languages, download.Language)
	}
	sort.Strings(languages)
	return languages
}

// offeredPlatforms lists the platforms a game has installers for, in the order
// they are offered everywhere else in the app.
func offeredPlatforms(game client.Game) []string {
	has := make(map[string]bool)
	for _, download := range game.Downloads {
		if len(download.Platforms.Windows) > 0 {
			has["Windows"] = true
		}
		if len(download.Platforms.Mac) > 0 {
			has["Mac"] = true
		}
		if len(download.Platforms.Linux) > 0 {
			has["Linux"] = true
		}
	}

	platforms := make([]string, 0, 3)
	for _, platform := range []string{"Windows", "Mac", "Linux"} {
		if has[platform] {
			platforms = append(platforms, platform)
		}
	}
	return platforms
}

// appendIf adds a fact only when GOG has something to say.
func appendIf(details []gameDetail, label, value string) []gameDetail {
	if strings.TrimSpace(value) == "" {
		return details
	}
	return append(details, gameDetail{Label: label, Value: value})
}

// renderStoreHeader is the description GOG publishes, in full. The pane it sits
// in scrolls, so there is nothing to be gained by cutting it short and putting
// the rest behind a button. It returns nil for a game GOG no longer describes,
// so the overview gives the space to the pictures instead.
func renderStoreHeader(summary string) fyne.CanvasObject {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}

	text := widget.NewLabel(summary)
	text.Wrapping = fyne.TextWrapWord
	return container.NewVBox(text)
}

// factsTwoColumnWidth is the width from which the facts are worth splitting in
// two. Below it the values have no room to say anything.
const factsTwoColumnWidth = 520

// renderGameDetails lays the facts out as forms of copyable values, in two
// columns when the pane is wide enough for them. Fifteen facts in one column is
// a wall; the same fifteen in two is a page.
func renderGameDetails(details []gameDetail) fyne.CanvasObject {
	if len(details) < 4 {
		return factsForm(details)
	}
	half := (len(details) + 1) / 2
	return newFactsGrid(factsForm(details[:half]), factsForm(details[half:]))
}

func factsForm(details []gameDetail) *widget.Form {
	items := make([]*widget.FormItem, 0, len(details))
	for _, detail := range details {
		value := NewCopyableLabel(detail.Value)
		value.Wrapping = fyne.TextWrapWord
		items = append(items, widget.NewFormItem(detail.Label, value))
	}
	return widget.NewForm(items...)
}

// factsGrid puts its two halves side by side when there is room, and one above
// the other when there is not. It is a widget rather than a layout because how
// tall it needs to be depends on how wide it has been made, and a layout is
// asked for its size before it is given one.
type factsGrid struct {
	widget.BaseWidget

	halves []fyne.CanvasObject
	// width is what the grid was last laid out at, which decides the shape it
	// reports next time it is asked.
	width float32
}

func newFactsGrid(halves ...fyne.CanvasObject) *factsGrid {
	grid := &factsGrid{halves: halves}
	grid.ExtendBaseWidget(grid)
	return grid
}

func (g *factsGrid) sideBySide() bool { return g.width >= factsTwoColumnWidth }

func (g *factsGrid) CreateRenderer() fyne.WidgetRenderer {
	return &factsGridRenderer{grid: g}
}

type factsGridRenderer struct {
	grid *factsGrid
}

func (r *factsGridRenderer) Layout(size fyne.Size) {
	r.grid.width = size.Width

	if !r.grid.sideBySide() {
		top := float32(0)
		for _, half := range r.grid.halves {
			height := half.MinSize().Height
			half.Move(fyne.NewPos(0, top))
			half.Resize(fyne.NewSize(size.Width, height))
			top += height + theme.Padding()
		}
		return
	}

	width := (size.Width - theme.Padding()) / 2
	for i, half := range r.grid.halves {
		half.Move(fyne.NewPos(float32(i)*(width+theme.Padding()), 0))
		half.Resize(fyne.NewSize(width, half.MinSize().Height))
	}
}

func (r *factsGridRenderer) MinSize() fyne.Size {
	min := fyne.NewSize(0, 0)
	for _, half := range r.grid.halves {
		size := half.MinSize()
		if r.grid.sideBySide() {
			// Side by side they are as tall as the taller one, and each needs
			// only half the width.
			min.Width += size.Width + theme.Padding()
			min.Height = fyne.Max(min.Height, size.Height)
			continue
		}
		min.Width = fyne.Max(min.Width, size.Width)
		min.Height += size.Height + theme.Padding()
	}
	return min
}

func (r *factsGridRenderer) Objects() []fyne.CanvasObject { return r.grid.halves }
func (r *factsGridRenderer) Refresh()                     { r.Layout(r.grid.Size()) }
func (r *factsGridRenderer) Destroy()                     {}

func filesWord(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}
