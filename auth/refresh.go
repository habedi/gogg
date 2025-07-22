package auth

import (
	"fmt"
	"time"

	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
)

// RefreshToken orchestrates the token refresh process.
// It retrieves the token from the database, checks its validity,
// and if expired, uses the client to get a new token from the GOG API,
// and finally updates the database.
func RefreshToken() (*db.Token, error) {
	token, err := db.GetTokenRecord()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token record: %w", err)
	}

	valid, err := isTokenValid(token)
	if err != nil {
		return nil, fmt.Errorf("failed to check token validity: %w", err)
	}

	if !valid {
		log.Info().Msg("Access token expired or invalid, refreshing...")
		newAccessToken, newRefreshToken, expiresIn, err := client.PerformTokenRefresh(token.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("failed to perform token refresh via client: %w", err)
		}

		token.AccessToken = newAccessToken
		token.RefreshToken = newRefreshToken
		token.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)

		if err := db.UpsertTokenRecord(token); err != nil {
			return nil, fmt.Errorf("failed to save refreshed token: %w", err)
		}
		log.Info().Msg("Token refreshed and saved successfully.")
	}

	return token, nil
}

// isTokenValid checks if the access token is still valid.
func isTokenValid(token *db.Token) (bool, error) {
	if token == nil {
		return false, fmt.Errorf("token record does not exist in the database; please login first")
	}

	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresAt == "" {
		return false, nil // Token data is incomplete, needs refresh.
	}

	expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to parse expiration time: %s", token.ExpiresAt)
		return false, err
	}

	// Check if the token expires in the next 5 minutes to be safe.
	return time.Now().Add(5 * time.Minute).Before(expiresAt), nil
}
