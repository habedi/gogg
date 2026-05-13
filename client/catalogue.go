package client

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/pool"
	"github.com/rs/zerolog/log"
)

func embedBase() string {
	if v := strings.TrimSpace(os.Getenv("GOGG_EMBED_BASE")); v != "" {
		return v
	}
	return "https://embed.gog.com"
}

// VersionChange records a version difference detected during catalogue refresh.
type VersionChange struct {
	GameID     int
	Title      string
	OldVersion string // empty when the game was not previously in the catalogue
	NewVersion string // empty when the game was removed from the GOG account
}

// extractVersion returns the first non-empty version string found across all
// platform files in a game's downloads. Returns "" when none is present.
func extractVersion(g Game) string {
	for _, dl := range g.Downloads {
		for _, platforms := range [][]PlatformFile{
			dl.Platforms.Windows,
			dl.Platforms.Mac,
			dl.Platforms.Linux,
		} {
			for _, f := range platforms {
				if f.Version != nil && *f.Version != "" {
					return *f.Version
				}
			}
		}
	}
	return ""
}

// RefreshCatalogue fetches all owned game details from GOG and updates the local database via the provided repo.
// It reports progress via the progressCb callback, which receives a value from 0.0 to 1.0.
// It returns a slice of VersionChange describing games that are new, updated, or removed since the last refresh.
func RefreshCatalogue(
	ctx context.Context,
	authService *auth.Service,
	repo db.GameRepository,
	numWorkers int,
	progressCb func(float64),
) ([]VersionChange, error) {
	// Prefer context-aware token refresh to honor cancellations/timeouts
	token, err := authService.RefreshTokenCtx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	ownedURL := fmt.Sprintf("%s/user/data/games", embedBase())
	gameIDs, err := FetchAllOwnedGameIDs(ctx, token.AccessToken, ownedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owned game IDs: %w", err)
	}
	// Snapshot current catalogue before clearing so we can detect version changes.
	oldGames, listErr := repo.List(ctx)
	oldVersions := make(map[int]struct{ title, version string }, len(oldGames))
	if listErr == nil {
		for _, g := range oldGames {
			oldVersions[g.ID] = struct{ title, version string }{g.Title, g.Version}
		}
	}

	if len(gameIDs) == 0 {
		log.Info().Msg("No games found in the GOG account.")
		if progressCb != nil {
			progressCb(1.0)
		}
		// All previously-owned games were removed.
		var changes []VersionChange
		for id, ov := range oldVersions {
			changes = append(changes, VersionChange{GameID: id, Title: ov.title, OldVersion: ov.version})
		}
		return changes, nil
	}

	if err := repo.Clear(ctx); err != nil {
		return nil, fmt.Errorf("failed to empty catalogue: %w", err)
	}

	var (
		processedCount atomic.Int64
		totalGames     = float64(len(gameIDs))

		mu          sync.Mutex
		newVersions = make(map[int]struct{ title, version string }, len(gameIDs))
	)

	workerFunc := func(ctx context.Context, id int) error {
		defer func() {
			count := processedCount.Add(1)
			if progressCb != nil {
				progressCb(float64(count) / totalGames)
			}
		}()

		url := fmt.Sprintf("%s/account/gameDetails/%d.json", embedBase(), id)
		details, raw, fetchErr := FetchGameData(ctx, token.AccessToken, url)
		if fetchErr != nil {
			log.Warn().Err(fetchErr).Int("gameID", id).Msg("Failed to fetch game details")
			return nil
		}
		if details.Title == "" {
			return nil
		}

		version := extractVersion(details)
		_ = repo.Put(ctx, db.Game{ID: id, Title: details.Title, Data: raw, Version: version})

		mu.Lock()
		newVersions[id] = struct{ title, version string }{details.Title, version}
		mu.Unlock()

		return nil
	}

	_ = pool.Run(ctx, gameIDs, numWorkers, workerFunc)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Build set of newly-owned IDs for removal detection.
	ownedSet := make(map[int]struct{}, len(gameIDs))
	for _, id := range gameIDs {
		ownedSet[id] = struct{}{}
	}

	var changes []VersionChange

	// Detect added and updated games.
	for id, nv := range newVersions {
		ov, existed := oldVersions[id]
		switch {
		case !existed:
			changes = append(changes, VersionChange{GameID: id, Title: nv.title, NewVersion: nv.version})
		case ov.version != nv.version:
			changes = append(changes, VersionChange{GameID: id, Title: nv.title, OldVersion: ov.version, NewVersion: nv.version})
		}
	}

	// Detect removed games (were in old catalogue, not in new owned set).
	for id, ov := range oldVersions {
		if _, owned := ownedSet[id]; !owned {
			changes = append(changes, VersionChange{GameID: id, Title: ov.title, OldVersion: ov.version})
		}
	}

	return changes, nil
}
