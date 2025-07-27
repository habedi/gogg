package operations

import (
	"encoding/json"
	"fmt"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// EstimationParams contains all parameters for calculating storage size.
type EstimationParams struct {
	LanguageCode  string
	PlatformName  string
	IncludeExtras bool
	IncludeDLCs   bool
}

// EstimateGameSize retrieves a game by ID and calculates its estimated download size.
func EstimateGameSize(gameID int, params EstimationParams) (int64, *client.Game, error) {
	game, err := db.GetGameByID(gameID)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to retrieve game data for ID %d: %w", gameID, err)
	}
	if game == nil {
		return 0, nil, fmt.Errorf("game with ID %d not found in the catalogue", gameID)
	}

	var nestedData client.Game
	if err := json.Unmarshal([]byte(game.Data), &nestedData); err != nil {
		return 0, nil, fmt.Errorf("failed to unmarshal game data for ID %d: %w", gameID, err)
	}

	langFullName, ok := client.GameLanguages[params.LanguageCode]
	if !ok {
		return 0, &nestedData, fmt.Errorf("invalid language code: %s", params.LanguageCode)
	}

	totalSizeBytes, err := nestedData.EstimateStorageSize(langFullName, params.PlatformName, params.IncludeExtras, params.IncludeDLCs)
	if err != nil {
		return 0, &nestedData, fmt.Errorf("failed to calculate storage size: %w", err)
	}

	return totalSizeBytes, &nestedData, nil
}
