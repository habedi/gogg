package client

import "encoding/json"

// Game contains information about a game and its downloadable content like extras and DLCs.
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

// Extra contains information about an extra file like game manual and soundtracks.
type Extra struct {
	Name      string `json:"name"`
	Size      string `json:"size"`
	ManualURL string `json:"manualUrl"`
}

// DLC contains information about a downloadable content like expansions and updates.
type DLC struct {
	Title           string          `json:"title"`
	BackgroundImage *string         `json:"backgroundImage,omitempty"`
	Downloads       [][]interface{} `json:"downloads"`
	Extras          []Extra         `json:"extras"`
	ParsedDownloads []Downloadable  `json:"-"`
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

// UnmarshalJSON is a custom unmarshal function for Game to process downloads and DLCs correctly.
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

	// Process RawDownloads for Game.
	gd.Downloads = parseRawDownloads(aux.RawDownloads)

	// Process DLC downloads.
	for i, dlc := range gd.DLCs {
		parsedDLCDownloads := parseRawDownloads(dlc.Downloads)
		gd.DLCs[i].ParsedDownloads = parsedDLCDownloads
	}

	return nil
}

// parseRawDownloads parses the raw downloads data into a slice of Downloadable.
func parseRawDownloads(rawDownloads [][]interface{}) []Downloadable {
	var downloads []Downloadable

	for _, raw := range rawDownloads {
		if len(raw) != 2 {
			continue
		}

		// First element is the language.
		language, ok := raw[0].(string)
		if !ok {
			continue
		}

		// Second element is the platforms object.
		platforms, err := parsePlatforms(raw[1])
		if err != nil {
			continue
		}

		downloads = append(downloads, Downloadable{
			Language:  language,
			Platforms: platforms,
		})
	}

	return downloads
}

// parsePlatforms parses the platforms data from an interface{}.
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
