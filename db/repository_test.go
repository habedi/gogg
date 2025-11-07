package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func TestGameRepositoryBasicCRUD(t *testing.T) {
	temp := t.TempDir()
	db.Path = filepath.Join(temp, "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })

	repo := db.NewGameRepository(db.GetDB())
	ctx := context.Background()

	// Put
	require.NoError(t, repo.Put(ctx, db.Game{ID: 1, Title: "Test Game", Data: "{}"}))

	// GetByID
	g, err := repo.GetByID(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, g)

	// List
	all, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)

	// Search
	res, err := repo.SearchByTitle(ctx, "Test")
	require.NoError(t, err)
	require.Len(t, res, 1)

	// Clear
	require.NoError(t, repo.Clear(ctx))
	all, err = repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 0)
}

func TestTokenRepositoryUpsertAndGet(t *testing.T) {
	temp := t.TempDir()
	db.Path = filepath.Join(temp, "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })

	repo := db.NewTokenRepository(db.GetDB())
	ctx := context.Background()

	// Initially empty
	tok, err := repo.Get(ctx)
	require.NoError(t, err)
	require.Nil(t, tok)

	// Upsert
	require.NoError(t, repo.Upsert(ctx, &db.Token{AccessToken: "a", RefreshToken: "r", ExpiresAt: "soon"}))

	// Retrieve
	tok, err = repo.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, tok)
	require.Equal(t, "a", tok.AccessToken)
}
