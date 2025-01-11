package client_test

import (
	"encoding/json"
	"github.com/habedi/gogg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func UnmarshalGameData(t *testing.T, jsonData string) *client.Game {
	var game client.Game
	err := json.Unmarshal([]byte(jsonData), &game)
	require.NoError(t, err)
	return &game
}

func TestParsesDownloadsCorrectly(t *testing.T) {
	jsonData := `{
		"title": "Test Game",
        "backgroundImage": "https://example.com/background.jpg",
		"downloads": [
			["English", {"windows": [{"name": "setup.exe", "size": "1GB"}]}]
		],
		"extras": [],
		"dlcs": []
	}`
	game := UnmarshalGameData(t, jsonData)

	assert.Equal(t, "Test Game", game.Title)
	assert.Len(t, game.Downloads, 1)
	assert.Equal(t, "English", game.Downloads[0].Language)
	assert.Len(t, game.Downloads[0].Platforms.Windows, 1)
	assert.Equal(t, "setup.exe", game.Downloads[0].Platforms.Windows[0].Name)
	assert.Equal(t, "1GB", game.Downloads[0].Platforms.Windows[0].Size)
}

func TestParsesDLCsCorrectly(t *testing.T) {
	jsonData := `{
		"title": "Test Game",
		"downloads": [],
		"extras": [],
		"dlcs": [
			{
				"title": "Test DLC",
				"downloads": [
					["English", {"windows": [{"name": "dlc_setup.exe", "size": "500MB"}]}]
				]
			}
		]
	}`
	game := UnmarshalGameData(t, jsonData)

	assert.Len(t, game.DLCs, 1)
	assert.Equal(t, "Test DLC", game.DLCs[0].Title)
	assert.Len(t, game.DLCs[0].ParsedDownloads, 1)
	assert.Equal(t, "English", game.DLCs[0].ParsedDownloads[0].Language)
	assert.Len(t, game.DLCs[0].ParsedDownloads[0].Platforms.Windows, 1)
	assert.Equal(t, "dlc_setup.exe", game.DLCs[0].ParsedDownloads[0].Platforms.Windows[0].Name)
	assert.Equal(t, "500MB", game.DLCs[0].ParsedDownloads[0].Platforms.Windows[0].Size)
}

func TestIgnoresInvalidDownloads(t *testing.T) {
	jsonData := `{
		"title": "Test Game",
		"downloads": [
			["English", {"windows": [{"name": "setup.exe", "size": "1GB"}]}],
			["Invalid"]
		],
		"extras": [],
		"dlcs": []
	}`
	game := UnmarshalGameData(t, jsonData)

	assert.Len(t, game.Downloads, 1)
	assert.Equal(t, "English", game.Downloads[0].Language)
	assert.Len(t, game.Downloads[0].Platforms.Windows, 1)
	assert.Equal(t, "setup.exe", game.Downloads[0].Platforms.Windows[0].Name)
	assert.Equal(t, "1GB", game.Downloads[0].Platforms.Windows[0].Size)
}

func TestParsesExtrasCorrectly(t *testing.T) {
	jsonData := `{
		"title": "Test Game",
		"downloads": [],
		"extras": [
			{"name": "Soundtrack", "size": "200MB", "manualUrl": "http://example.com/soundtrack"}
		],
		"dlcs": []
	}`
	game := UnmarshalGameData(t, jsonData)

	assert.Len(t, game.Extras, 1)
	assert.Equal(t, "Soundtrack", game.Extras[0].Name)
	assert.Equal(t, "200MB", game.Extras[0].Size)
	assert.Equal(t, "http://example.com/soundtrack", game.Extras[0].ManualURL)
}

func TestHandlesEmptyDownloads(t *testing.T) {
	jsonData := `{
		"title": "Test Game",
		"downloads": [],
		"extras": [],
		"dlcs": []
	}`
	game := UnmarshalGameData(t, jsonData)

	assert.Equal(t, "Test Game", game.Title)
	assert.Empty(t, game.Downloads)
	assert.Empty(t, game.Extras)
	assert.Empty(t, game.DLCs)
}
