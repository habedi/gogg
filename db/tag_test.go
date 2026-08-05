package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func tagDB(t *testing.T) {
	t.Helper()
	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })
}

// Marking a game and asking for it back is the whole point.
func TestTags_MarkAndUnmark(t *testing.T) {
	tagDB(t)
	ctx := context.Background()

	require.NoError(t, db.AddTag(ctx, 1, db.TagFavorite))
	require.NoError(t, db.AddTag(ctx, 1, db.TagHidden))

	tags, err := db.TagsFor(ctx, 1)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{db.TagFavorite, db.TagHidden}, tags)

	require.NoError(t, db.RemoveTag(ctx, 1, db.TagHidden))
	tags, err = db.TagsFor(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, []string{db.TagFavorite}, tags)
}

// Clicking Favourite twice must not fail, and must not mark it twice.
func TestTags_MarkingTwiceIsNotAnError(t *testing.T) {
	tagDB(t)
	ctx := context.Background()

	require.NoError(t, db.AddTag(ctx, 1, db.TagFavorite))
	require.NoError(t, db.AddTag(ctx, 1, db.TagFavorite))

	tags, err := db.TagsFor(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, []string{db.TagFavorite}, tags)
}

// Unmarking something that was never marked is what a toggle does half the time.
func TestTags_UnmarkingWhatIsNotThere(t *testing.T) {
	tagDB(t)
	require.NoError(t, db.RemoveTag(context.Background(), 42, db.TagHidden))
}

// One spelling, whatever the user typed.
func TestTags_AreCaseInsensitive(t *testing.T) {
	tagDB(t)
	ctx := context.Background()

	require.NoError(t, db.AddTag(ctx, 1, "  Favorite "))
	tags, err := db.TagsFor(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, []string{db.TagFavorite}, tags)

	require.NoError(t, db.RemoveTag(ctx, 1, "FAVORITE"))
	tags, _ = db.TagsFor(ctx, 1)
	require.Empty(t, tags)
}

func TestTags_EmptyTagIsRefused(t *testing.T) {
	tagDB(t)
	require.Error(t, db.AddTag(context.Background(), 1, "   "))
}

// The library reads every tag at once, because it asks about every game.
func TestAllTags(t *testing.T) {
	tagDB(t)
	ctx := context.Background()

	require.NoError(t, db.AddTag(ctx, 1, db.TagFavorite))
	require.NoError(t, db.AddTag(ctx, 2, db.TagHidden))
	require.NoError(t, db.AddTag(ctx, 2, "rpg"))

	all, err := db.AllTags(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{db.TagFavorite}, all[1])
	require.ElementsMatch(t, []string{db.TagHidden, "rpg"}, all[2])
	require.Empty(t, all[3])
}

// A catalogue from a gogg that knew nothing about tags still opens.
func TestTags_OlderCatalogueOpensAndCanBeTagged(t *testing.T) {
	tagDB(t)
	ctx := context.Background()

	require.NoError(t, db.PutInGame(1, "Game 1", `{"title":"G"}`))
	all, err := db.AllTags(ctx)
	require.NoError(t, err)
	require.Empty(t, all)

	require.NoError(t, db.AddTag(ctx, 1, db.TagFavorite))
	all, err = db.AllTags(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
}
