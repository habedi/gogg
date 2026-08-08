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

// factsOf reads a game's stored data, or remembers what it said last time. A
// query is asked of every game on every keystroke, and reading the whole
// catalogue for each letter typed is what made searching a large library
// crawl.
func (s *libraryState) factsOf(game db.Game) gameFacts {
	if facts, ok := s.facts[game.ID]; ok && facts.data == game.Data {
		return facts
	}

	facts := gameFacts{data: game.Data}
	if parsed, err := parseGameData(game.Data); err == nil {
		for _, platform := range offeredPlatforms(parsed) {
			facts.platforms = append(facts.platforms, strings.ToLower(platform))
		}
		facts.languages = languageNamesAndCodes(offeredLanguages(parsed))
	}
	s.facts[game.ID] = facts
	return facts
}

// forgetParsed drops what was read of the catalogue, because it is no longer
// the same catalogue.
func (s *libraryState) forgetParsed() {
	s.facts = make(map[int]gameFacts)
	s.sizes = make(map[sizeCacheKey]int64)
	s.catalogueSizes = make(map[int]int64)
}

// downloadable reports whether GOG serves any installer files for a game.
// One it does not, such as an online, Galaxy-only title, cannot be
// downloaded, so it cannot be selected for one either.
func (s *libraryState) downloadable(game db.Game) bool {
	return len(s.factsOf(game).platforms) > 0
}

// catalogueSize is a game's size for filtering: the largest single-platform
// install it offers, in any language, extras and DLCs counted. It does not
// read the download-form settings, so filtering by size is about the game
// rather than about the platform the user happens to have picked.
func (s *libraryState) catalogueSize(game db.Game) int64 {
	if size, ok := s.catalogueSizes[game.ID]; ok {
		return size
	}
	parsed, err := parseGameData(game.Data)
	if err != nil {
		s.catalogueSizes[game.ID] = 0
		return 0
	}
	languages := make(map[string]bool)
	for _, download := range parsed.Downloads {
		languages[download.Language] = true
	}

	var largest int64
	for language := range languages {
		for _, platform := range []string{"windows", "mac", "linux"} {
			if size, err := parsed.EstimateStorageSize(language, platform, true, true); err == nil && size > largest {
				largest = size
			}
		}
	}
	s.catalogueSizes[game.ID] = largest
	return largest
}

// sizeCacheKey identifies an estimate together with the settings it was
// computed under, so changing any of them yields a fresh estimate.
type sizeCacheKey struct {
	id             int
	lang, platform string
	extras, dlcs   bool
}

func (s *libraryState) estimateSize(game db.Game) int64 {
	prefs := fyne.CurrentApp().Preferences()
	key := sizeCacheKey{
		id:       game.ID,
		lang:     formLanguage(prefs),
		platform: formPlatform(prefs),
		extras:   formExtras(prefs),
		dlcs:     formDLCs(prefs),
	}
	if v, ok := s.sizes[key]; ok {
		return v
	}
	parsed, err := parseGameData(game.Data)
	if err != nil {
		s.sizes[key] = 0
		return 0
	}
	// The stored game data names languages in full, not by code.
	langFullName, ok := client.GameLanguages[key.lang]
	if !ok {
		s.sizes[key] = 0
		return 0
	}
	sz, err := parsed.EstimateStorageSize(langFullName, key.platform, key.extras, key.dlcs)
	if err != nil {
		s.sizes[key] = 0
		return 0
	}
	s.sizes[key] = sz
	return sz
}

// loadTags reads what the user has marked games with. It is read in one go
// and kept, because a query is asked of every game on every keystroke.
func (s *libraryState) loadTags(store db.TagRepository) {
	tags, err := store.All(context.Background())
	if err != nil {
		log.Debug().Err(err).Msg("Could not read game tags")
		return
	}
	s.tags = tags
}

// loadGenres reads the genres out of the stored lookups.
func (s *libraryState) loadGenres(store db.MetadataRepository) {
	records, err := store.All(context.Background())
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
	s.genres = genres
}

// hasTag says whether a game carries a tag, read from the cache the searches
// read.
func (s *libraryState) hasTag(gameID int, tag string) bool {
	for _, held := range s.tags[gameID] {
		if held == tag {
			return true
		}
	}
	return false
}

// factsFor describes a game to a query. Working out the size or the platforms
// means parsing the stored data, so it is only done for queries that ask.
func (s *libraryState) factsFor(game db.Game, needs search.Needs) search.Facts {
	facts := search.Facts{Title: game.Title}
	if status, ok := s.statuses[game.ID]; ok {
		facts.Downloaded = status.Downloaded
		facts.HasUpdate = status.HasUpdate
		facts.UpdatedAt = status.ChangedAt
	}
	if needs.Tags {
		facts.Tags = s.tags[game.ID]
	}
	if needs.Genres {
		facts.Genres = s.genres[game.ID]
	}
	if needs.Size {
		// The filter asks about the game's size, not the size of what the
		// current settings would download, so it uses the catalogue size.
		facts.SizeBytes = s.catalogueSize(game)
	}
	if needs.Platforms || needs.Languages {
		read := s.factsOf(game)
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
