package gui

import (
	"context"
	"encoding/json"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/search"
	"github.com/rs/zerolog/log"
)

// parseGameData reads a game's stored data. It is a variable so tests can count
// how often the catalogue is read.
var parseGameData = client.ParseGameData

// gameFacts is what a query asks about a game that costs something to answer:
// reading its stored data. The data it was read from is kept alongside, so a
// game whose data has changed is read again rather than answered from what it
// used to say.
type gameFacts struct {
	data      string
	platforms []string
	languages []string
}

// parsedFacts keeps what each game's data said. A query is asked of every game
// on every keystroke, and reading the whole catalogue for each letter typed is
// what made searching a large library crawl.
var parsedFacts = map[int]gameFacts{}

// factsOf reads a game's stored data, or remembers what it said last time.
func factsOf(game db.Game) gameFacts {
	if facts, ok := parsedFacts[game.ID]; ok && facts.data == game.Data {
		return facts
	}

	facts := gameFacts{data: game.Data}
	if parsed, err := parseGameData(game.Data); err == nil {
		for _, platform := range offeredPlatforms(parsed) {
			facts.platforms = append(facts.platforms, strings.ToLower(platform))
		}
		facts.languages = languageNamesAndCodes(offeredLanguages(parsed))
	}
	parsedFacts[game.ID] = facts
	return facts
}

// forgetParsedGames drops what was read of the catalogue, because it is no
// longer the same catalogue.
func forgetParsedGames() {
	parsedFacts = make(map[int]gameFacts)
	sizeCache = make(map[sizeCacheKey]int64)
}

// sizeCacheKey identifies an estimate together with the settings it was
// computed under, so changing any of them yields a fresh estimate.
type sizeCacheKey struct {
	id             int
	lang, platform string
	extras, dlcs   bool
}

var sizeCache = make(map[sizeCacheKey]int64)

func estimateGameSize(game db.Game) int64 {
	prefs := fyne.CurrentApp().Preferences()
	key := sizeCacheKey{
		id:       game.ID,
		lang:     prefs.StringWithFallback("downloadForm.language", "en"),
		platform: prefs.StringWithFallback("downloadForm.platform", "windows"),
		extras:   prefs.BoolWithFallback("downloadForm.extras", true),
		dlcs:     prefs.BoolWithFallback("downloadForm.dlcs", true),
	}
	if v, ok := sizeCache[key]; ok {
		return v
	}
	parsed, err := parseGameData(game.Data)
	if err != nil {
		sizeCache[key] = 0
		return 0
	}
	// The stored game data names languages in full, not by code.
	langFullName, ok := client.GameLanguages[key.lang]
	if !ok {
		sizeCache[key] = 0
		return 0
	}
	sz, err := parsed.EstimateStorageSize(langFullName, key.platform, key.extras, key.dlcs)
	if err != nil {
		sizeCache[key] = 0
		return 0
	}
	sizeCache[key] = sz
	return sz
}

// gameTags is what the user has marked games with, read once per refresh
// because a query is asked of every game on every keystroke.
var gameTags = map[int][]string{}

// loadGameTags reads what the user has marked games with. It is read in one go
// and kept, because a query is asked of every game on every keystroke.
func loadGameTags() {
	tags, err := db.AllTags(context.Background())
	if err != nil {
		log.Debug().Err(err).Msg("Could not read game tags")
		return
	}
	gameTags = tags
}

// gameGenres is what GOG's store says each game is, for games whose store
// pages have been looked up. Read in one go and kept, like the tags.
var gameGenres = map[int][]string{}

// loadGameGenres reads the genres out of the stored lookups.
func loadGameGenres() {
	records, err := db.AllGameMetadata(context.Background())
	if err != nil {
		log.Debug().Err(err).Msg("Could not read stored metadata")
		return
	}

	genres := make(map[int][]string, len(records))
	for _, record := range records {
		var meta client.GameMetadata
		if json.Unmarshal(record.Data, &meta) != nil || len(meta.Genres) == 0 {
			continue
		}
		genres[record.GameID] = meta.Genres
	}
	gameGenres = genres
}

// gameHasTag says whether a game carries a tag, read from the cache the
// searches read.
func gameHasTag(gameID int, tag string) bool {
	for _, held := range gameTags[gameID] {
		if held == tag {
			return true
		}
	}
	return false
}

// factsFor describes a game to a query. Working out the size or the platforms
// means parsing the stored data, so it is only done for queries that ask.
func factsFor(game db.Game, needs search.Needs) search.Facts {
	facts := search.Facts{Title: game.Title}
	if status, ok := updateStatusCache[game.ID]; ok {
		facts.Downloaded = status.Downloaded
		facts.HasUpdate = status.HasUpdate
		facts.UpdatedAt = status.ChangedAt
	}
	if needs.Tags {
		facts.Tags = gameTags[game.ID]
	}
	if needs.Genres {
		facts.Genres = gameGenres[game.ID]
	}
	if needs.Size {
		facts.SizeBytes = estimateGameSize(game)
	}
	if needs.Platforms || needs.Languages {
		read := factsOf(game)
		if needs.Platforms {
			facts.Platforms = read.platforms
		}
		if needs.Languages {
			facts.Languages = read.languages
		}
	}
	return facts
}

// languageNamesAndCodes lets a game be asked for either way: lang:german and
// lang:de mean the same thing to someone typing quickly.
func languageNamesAndCodes(names []string) []string {
	both := make([]string, 0, len(names)*2)
	for _, name := range names {
		both = append(both, name)
		for code, full := range client.GameLanguages {
			if strings.EqualFold(full, name) {
				both = append(both, code)
				break
			}
		}
	}
	return both
}
