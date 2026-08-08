package db_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func metadataDB(t *testing.T) {
	t.Helper()
	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })
}

// Storing a lookup and reading it back is the whole point.
func TestGameMetadata_StoreAndRead(t *testing.T) {
	metadataDB(t)
	ctx := context.Background()

	require.NoError(t, db.PutGameMetadata(ctx, 7, 2, []byte(`{"summary":"a game"}`)))

	record, err := db.GetGameMetadata(ctx, 7)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, 7, record.GameID)
	require.Equal(t, 2, record.Format)
	require.JSONEq(t, `{"summary":"a game"}`, string(record.Data))
	require.WithinDuration(t, time.Now(), record.FetchedAt, time.Minute)
}

// A second lookup replaces the first: there is one answer per game.
func TestGameMetadata_SecondStoreReplacesTheFirst(t *testing.T) {
	metadataDB(t)
	ctx := context.Background()

	require.NoError(t, db.PutGameMetadata(ctx, 7, 1, []byte(`{"summary":"old"}`)))
	require.NoError(t, db.PutGameMetadata(ctx, 7, 2, []byte(`{"summary":"new"}`)))

	record, err := db.GetGameMetadata(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, 2, record.Format)
	require.JSONEq(t, `{"summary":"new"}`, string(record.Data))
}

// A game never looked up is nil rather than an error: absence is a normal
// answer, not a failure.
func TestGameMetadata_NothingStoredIsNil(t *testing.T) {
	metadataDB(t)

	record, err := db.GetGameMetadata(context.Background(), 999)
	require.NoError(t, err)
	require.Nil(t, record)
}

// The fetch time can be named, so a record can say it is older than the
// moment it was written.
func TestGameMetadata_StoreAtKeepsTheNamedTime(t *testing.T) {
	metadataDB(t)
	ctx := context.Background()

	past := time.Now().Add(-90 * 24 * time.Hour).Truncate(time.Second)
	require.NoError(t, db.PutGameMetadataAt(ctx, 7, 2, []byte(`{}`), past))

	record, err := db.GetGameMetadata(ctx, 7)
	require.NoError(t, err)
	require.WithinDuration(t, past, record.FetchedAt, time.Second)
}

// A database that was never opened answers with an error, not a panic.
func TestGameMetadata_UninitializedDatabaseErrors(t *testing.T) {
	require.NoError(t, db.CloseDB())

	require.Error(t, db.PutGameMetadata(context.Background(), 7, 2, []byte(`{}`)))
	_, err := db.GetGameMetadata(context.Background(), 7)
	require.Error(t, err)
}
