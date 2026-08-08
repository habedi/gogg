package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func TestGameRepositoryDeleteByIDs(t *testing.T) {
	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })

	repo := db.NewGameRepository(db.GetDB())
	ctx := context.Background()

	for _, id := range []int{1, 2, 3} {
		require.NoError(t, repo.Put(ctx, db.Game{ID: id, Title: "Game", Data: "{}"}))
	}

	require.NoError(t, repo.DeleteByIDs(ctx, []int{1, 3}))

	all, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, 2, all[0].ID)

	// Deleting nothing is a no-op, not an error.
	require.NoError(t, repo.DeleteByIDs(ctx, nil))
	all, err = repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
}
