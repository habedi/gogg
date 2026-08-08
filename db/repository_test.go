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

func TestTagRepository_CRUD(t *testing.T) {
	temp := t.TempDir()
	db.Path = filepath.Join(temp, "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })

	repo := db.NewTagRepository(db.GetDB())
	ctx := context.Background()

	require.NoError(t, repo.Add(ctx, 1, db.TagFavorite))
	require.NoError(t, repo.Add(ctx, 1, db.TagHidden))
	require.NoError(t, repo.Add(ctx, 2, db.TagFavorite))
	// Adding the same tag twice is not an error.
	require.NoError(t, repo.Add(ctx, 1, db.TagFavorite))
	// An empty tag is refused.
	require.Error(t, repo.Add(ctx, 1, "   "))

	all, err := repo.All(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{db.TagFavorite, db.TagHidden}, all[1])
	require.Equal(t, []string{db.TagFavorite}, all[2])

	require.NoError(t, repo.Remove(ctx, 1, db.TagHidden))
	all, err = repo.All(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{db.TagFavorite}, all[1], "the removed tag is gone, the other stays")
}

func TestMetadataRepository_CRUD(t *testing.T) {
	temp := t.TempDir()
	db.Path = filepath.Join(temp, "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })

	repo := db.NewMetadataRepository(db.GetDB())
	ctx := context.Background()

	// A game with no record yet reads as nil, not an error.
	got, err := repo.Get(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, got)

	require.NoError(t, repo.Put(ctx, 1, 3, []byte(`{"summary":"a game"}`)))
	got, err = repo.Get(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, 3, got.Format)
	require.JSONEq(t, `{"summary":"a game"}`, string(got.Data))

	// Put again upserts rather than duplicating.
	require.NoError(t, repo.Put(ctx, 1, 4, []byte(`{"summary":"updated"}`)))
	got, err = repo.Get(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 4, got.Format)

	require.NoError(t, repo.Put(ctx, 2, 3, []byte(`{}`)))
	records, err := repo.All(ctx)
	require.NoError(t, err)
	require.Len(t, records, 2)

	// The package-level accessor reads the same rows.
	viaGlobal, err := db.AllGameMetadata(ctx)
	require.NoError(t, err)
	require.Len(t, viaGlobal, 2)
}
