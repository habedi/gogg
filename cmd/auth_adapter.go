package cmd

import (
	"context"

	"github.com/habedi/gogg/db"
)

// tokenRepoStorer adapts a TokenRepository to the auth.TokenStorer interface.
type tokenRepoStorer struct{ repo db.TokenRepository }

func (s *tokenRepoStorer) GetTokenRecord() (*db.Token, error) {
	return s.repo.Get(context.Background())
}

func (s *tokenRepoStorer) UpsertTokenRecord(token *db.Token) error {
	return s.repo.Upsert(context.Background(), token)
}
