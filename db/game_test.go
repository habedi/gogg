package db_test

import (
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDBForGames sets up an in-memory SQLite database for testing purposes.
// It returns a pointer to the gorm.DB instance.
func setupTestDBForGames(t *testing.T) *gorm.DB {
	dBOject, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, dBOject.AutoMigrate(&db.Game{}))
	return dBOject
}

// TestPutInGame_InsertsNewGame tests the insertion of a new game into the database.
func TestPutInGame_InsertsNewGame(t *testing.T) {
	testDB := setupTestDBForGames(t)
	db.Db = testDB

	err := db.PutInGame(1, "Test Game", "Test Data")
	require.NoError(t, err)

	var game db.Game
	err = testDB.First(&game, 1).Error
	require.NoError(t, err)
	assert.Equal(t, "Test Game", game.Title)
	assert.Equal(t, "Test Data", game.Data)
}

// TestPutInGame_UpdatesExistingGame tests the update of an existing game in the database.
func TestPutInGame_UpdatesExistingGame(t *testing.T) {
	testDB := setupTestDBForGames(t)
	db.Db = testDB

	err := db.PutInGame(1, "Test Game", "Test Data")
	require.NoError(t, err)

	err = db.PutInGame(1, "Updated Game", "Updated Data")
	require.NoError(t, err)

	var game db.Game
	err = testDB.First(&game, 1).Error
	require.NoError(t, err)
	assert.Equal(t, "Updated Game", game.Title)
	assert.Equal(t, "Updated Data", game.Data)
}

// TestEmptyCatalogue_RemovesAllGames tests the removal of all games from the database.
func TestEmptyCatalogue_RemovesAllGames(t *testing.T) {
	testDB := setupTestDBForGames(t)
	db.Db = testDB

	err := db.PutInGame(1, "Test Game", "Test Data")
	require.NoError(t, err)

	err = db.EmptyCatalogue()
	require.NoError(t, err)

	var games []db.Game
	err = testDB.Find(&games).Error
	require.NoError(t, err)
	assert.Empty(t, games)
}

// TestGetCatalogue_ReturnsAllGames tests the retrieval of all games from the database.
func TestGetCatalogue_ReturnsAllGames(t *testing.T) {
	testDB := setupTestDBForGames(t)
	db.Db = testDB

	err := db.PutInGame(1, "Test Game 1", "Test Data 1")
	require.NoError(t, err)
	err = db.PutInGame(2, "Test Game 2", "Test Data 2")
	require.NoError(t, err)

	games, err := db.GetCatalogue()
	require.NoError(t, err)
	assert.Len(t, games, 2)
}

// TestGetGameByID_ReturnsGame tests the retrieval of a game by its ID from the database.
func TestGetGameByID_ReturnsGame(t *testing.T) {
	testDB := setupTestDBForGames(t)
	db.Db = testDB

	err := db.PutInGame(1, "Test Game", "Test Data")
	require.NoError(t, err)

	game, err := db.GetGameByID(1)
	require.NoError(t, err)
	assert.NotNil(t, game)
	assert.Equal(t, "Test Game", game.Title)
	assert.Equal(t, "Test Data", game.Data)
}

// TestGetGameByID_ReturnsNilForNonExistentGame tests that a non-existent game returns nil.
func TestGetGameByID_ReturnsNilForNonExistentGame(t *testing.T) {
	testDB := setupTestDBForGames(t)
	db.Db = testDB

	game, err := db.GetGameByID(1)
	require.NoError(t, err)
	assert.Nil(t, game)
}

// TestSearchGamesByName_ReturnsMatchingGames tests the search functionality for games by name.
func TestSearchGamesByName_ReturnsMatchingGames(t *testing.T) {
	testDB := setupTestDBForGames(t)
	db.Db = testDB

	err := db.PutInGame(1, "Test Game 1", "Test Data 1")
	require.NoError(t, err)
	err = db.PutInGame(2, "Another Game", "Test Data 2")
	require.NoError(t, err)

	games, err := db.SearchGamesByName("Test")
	require.NoError(t, err)
	assert.Len(t, games, 1)
	assert.Equal(t, "Test Game 1", games[0].Title)
}

// TestSearchGamesByName_ReturnsEmptyForNoMatches tests that no matches return an empty result.
func TestSearchGamesByName_ReturnsEmptyForNoMatches(t *testing.T) {
	testDB := setupTestDBForGames(t)
	db.Db = testDB

	err := db.PutInGame(1, "Test Game 1", "Test Data 1")
	require.NoError(t, err)

	games, err := db.SearchGamesByName("Nonexistent")
	require.NoError(t, err)
	assert.Empty(t, games)
}
