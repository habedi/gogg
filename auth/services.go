package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
)

// Service orchestrates the token refresh process using its dependencies.
type Service struct {
	Storer    TokenStorer
	Refresher TokenRefresher
}

// NewService is the constructor for our auth service.
func NewService(storer TokenStorer, refresher TokenRefresher) *Service {
	return &Service{
		Storer:    storer,
		Refresher: refresher,
	}
}

// NewServiceWithRepo constructs Service using a TokenRepository directly.
func NewServiceWithRepo(tokenRepo db.TokenRepository, refresher TokenRefresher) *Service {
	adapter := &struct{ TokenStorer }{TokenStorer: &tokenRepoStorer{repo: tokenRepo}}
	return NewService(adapter.TokenStorer, refresher)
}

// RefreshToken is a method that handles the full token refresh logic.
// Deprecated: Use RefreshTokenCtx for context-aware cancellation support.
func (s *Service) RefreshToken() (*db.Token, error) {
	return s.RefreshTokenCtx(context.Background())
}

// RefreshTokenCtx performs refresh honoring cancellation if the refresher supports it.
func (s *Service) RefreshTokenCtx(ctx context.Context) (*db.Token, error) {
	token, err := s.Storer.GetTokenRecord()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token record: %w", err)
	}

	valid, err := isTokenValid(token)
	if err != nil {
		return nil, fmt.Errorf("failed to check token validity: %w", err)
	}
	if valid {
		return token, nil
	}

	var access, refresh string
	var expiresIn int64
	if rf, ok := s.Refresher.(TokenRefresherWithCtx); ok {
		access, refresh, expiresIn, err = rf.PerformTokenRefreshCtx(ctx, token.RefreshToken)
	} else {
		access, refresh, expiresIn, err = s.Refresher.PerformTokenRefresh(token.RefreshToken)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to perform token refresh via client: %w", err)
	}
	token.AccessToken = access
	token.RefreshToken = refresh
	token.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)
	if err := s.Storer.UpsertTokenRecord(token); err != nil {
		return nil, fmt.Errorf("failed to save refreshed token: %w", err)
	}
	log.Info().Msg("Token refreshed and saved successfully.")
	return token, nil
}

// isTokenValid checks if the access token is still valid.
func isTokenValid(token *db.Token) (bool, error) {
	if token == nil {
		return false, fmt.Errorf("token record does not exist in the database; please login first")
	}
	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresAt == "" {
		return false, nil
	}
	expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to parse expiration time: %s", token.ExpiresAt)
		return false, err
	}
	return time.Now().Add(5 * time.Minute).Before(expiresAt), nil
}

// tokenRepoStorer adapts db.TokenRepository to TokenStorer.
type tokenRepoStorer struct{ repo db.TokenRepository }

func (s *tokenRepoStorer) GetTokenRecord() (*db.Token, error) {
	return s.repo.Get(context.Background())
}
func (s *tokenRepoStorer) UpsertTokenRecord(token *db.Token) error {
	return s.repo.Upsert(context.Background(), token)
}
