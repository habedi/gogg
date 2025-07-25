package client

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/pool"
	"github.com/rs/zerolog/log"
)

// RefreshCatalogue fetches all owned game details from GOG and updates the local database.
// It reports progress via the progressCb callback, which receives a value from 0.0 to 1.0.
func RefreshCatalogue(
	ctx context.Context,
	authService *auth.Service,
	numWorkers int,
	progressCb func(float64),
) error {
	token, err := authService.RefreshToken()
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	gameIDs, err := FetchIdOfOwnedGames(token.AccessToken, "https://embed.gog.com/user/data/games")
	if err != nil {
		return fmt.Errorf("failed to fetch owned game IDs: %w", err)
	}
	if len(gameIDs) == 0 {
		log.Info().Msg("No games found in the GOG account.")
		if progressCb != nil {
			progressCb(1.0) // Signal completion
		}
		return nil
	}

	if err := db.EmptyCatalogue(); err != nil {
		return fmt.Errorf("failed to empty catalogue: %w", err)
	}

	var processedCount atomic.Int64
	totalGames := float64(len(gameIDs))

	workerFunc := func(ctx context.Context, id int) error {
		// Defer the counter increment to guarantee it runs even if a fetch fails.
		defer func() {
			count := processedCount.Add(1)
			if progressCb != nil {
				progress := float64(count) / totalGames
				progressCb(progress)
			}
		}()

		url := fmt.Sprintf("https://embed.gog.com/account/gameDetails/%d.json", id)
		details, raw, fetchErr := FetchGameData(token.AccessToken, url)
		if fetchErr != nil {
			log.Warn().Err(fetchErr).Int("gameID", id).Msg("Failed to fetch game details")
			return nil // Don't treat as a fatal error for the pool
		}
		if details.Title != "" {
			if err := db.PutInGame(id, details.Title, raw); err != nil {
				log.Error().Err(err).Int("gameID", id).Msg("Failed to save game to DB")
			}
		}

		return nil
	}

	_ = pool.Run(ctx, gameIDs, numWorkers, workerFunc)

	return ctx.Err()
}
