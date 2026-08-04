package gui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
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
func gameDetails(game db.Game, dm *DownloadManager) []gameDetail {
	details := []gameDetail{{Label: "Game ID", Value: strconv.Itoa(game.ID)}}

	version := game.Version
	if version == "" {
		version = "Unknown"
	}
	details = append(details, gameDetail{Label: "Version", Value: version})

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

// renderGameDetails lays the facts out as a form of copyable values.
func renderGameDetails(details []gameDetail) fyne.CanvasObject {
	items := make([]*widget.FormItem, 0, len(details))
	for _, detail := range details {
		value := NewCopyableLabel(detail.Value)
		value.Wrapping = fyne.TextWrapWord
		items = append(items, widget.NewFormItem(detail.Label, value))
	}
	return widget.NewForm(items...)
}

func filesWord(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}
