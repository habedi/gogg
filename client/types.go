package client

import (
	"encoding/json"

	"github.com/rs/zerolog/log"
)

// Game contains information about a game and its downloadable content.
type Game struct {
	Title           string         `json:"title"`
	BackgroundImage *string        `json:"backgroundImage,omitempty"`
	Downloads       []Downloadable `json:"downloads"`
	Extras          []Extra        `json:"extras"`
	DLCs            []DLC          `json:"dlcs"`
}

// PlatformFile contains information about a platform-specific installation file.
type PlatformFile struct {
	ManualURL *string `json:"manualUrl,omitempty"`
	Name      string  `json:"name"`
	Version   *string `json:"version,omitempty"`
	Date      *string `json:"date,omitempty"`
	Size      string  `json:"size"`
}

// Extra contains information about an extra file like a manual or soundtrack.
type Extra struct {
	Name      string `json:"name"`
	Size      string `json:"size"`
	ManualURL string `json:"manualUrl"`
}

// DLC contains information about downloadable content like expansions.
type DLC struct {
	Title           string          `json:"title"`
	BackgroundImage *string         `json:"backgroundImage,omitempty"`
	Downloads       [][]interface{} `json:"downloads"` // Keep raw for initial parsing
	Extras          []Extra         `json:"extras"`
	ParsedDownloads []Downloadable  `json:"-"` // Store parsed downloads here
}

// Platform contains information about platform-specific installation files.
type Platform struct {
	Windows []PlatformFile `json:"windows,omitempty"`
	Mac     []PlatformFile `json:"mac,omitempty"`
	Linux   []PlatformFile `json:"linux,omitempty"`
}

// Downloadable contains information about a downloadable file for a specific language and platform.
type Downloadable struct {
	Language  string   `json:"language"`
	Platforms Platform `json:"platforms"`
}

// UnmarshalJSON implements custom unmarshalling for Game to process downloads and DLCs.
func (gd *Game) UnmarshalJSON(data []byte) error {
	type Alias Game
	aux := &struct {
		RawDownloads [][]interface{} `json:"downloads"`
		*Alias
	}{
		Alias: (*Alias)(gd),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Process downloads for the game.
	gd.Downloads = parseRawDownloads(aux.RawDownloads)

	// Process downloads for each DLC.
	for i, dlc := range gd.DLCs {
		gd.DLCs[i].ParsedDownloads = parseRawDownloads(dlc.Downloads)
	}
	return nil
}

// parseRawDownloads converts raw downloads data into a slice of Downloadable.
func parseRawDownloads(rawDownloads [][]interface{}) []Downloadable {
	var downloads []Downloadable
	for _, raw := range rawDownloads {
		if len(raw) != 2 {
			continue // Expecting [language, platforms]
		}
		language, ok := raw[0].(string)
		if !ok {
			continue
		}
		platforms, err := parsePlatforms(raw[1])
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to parse platforms for language %s", language)
			continue
		}
		downloads = append(downloads, Downloadable{
			Language:  language,
			Platforms: platforms,
		})
	}
	return downloads
}

// parsePlatforms decodes the platform object.
func parsePlatforms(data interface{}) (Platform, error) {
	platformsData, err := json.Marshal(data)
	if err != nil {
		return Platform{}, err
	}
	var platforms Platform
	if err := json.Unmarshal(platformsData, &platforms); err != nil {
		return Platform{}, err
	}
	return platforms, nil
}

// ParseGameData parses raw game data JSON into a Game struct.
func ParseGameData(data string) (Game, error) {
	var rawResponse Game
	if err := json.Unmarshal([]byte(data), &rawResponse); err != nil {
		log.Error().Err(err).Msg("Failed to parse game data")
		return Game{}, err
	}
	return rawResponse, nil
}

// parseGameData specific helper used by FetchGameData
func parseGameDataInternal(body []byte, game *Game) error {
	if err := json.Unmarshal(body, game); err != nil {
		// Log the problematic body content (or part of it) for debugging if log level allows
		log.Error().Err(err).Str("body_preview", string(body[:min(len(body), 200)])).Msg("Failed to parse game data JSON")
		return err
	}
	return nil
}

// parseOwnedGames specific helper used by FetchIdOfOwnedGames
func parseOwnedGames(body []byte) ([]int, error) {
	var response struct {
		Owned []int `json:"owned"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		log.Error().Err(err).Str("body_preview", string(body[:min(len(body), 200)])).Msg("Failed to parse owned games JSON")
		return nil, err
	}
	return response.Owned, nil
}
