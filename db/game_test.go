package db_test

import (
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func setupTestDBForGames(t *testing.T) *gorm.DB {
	dBOject, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, dBOject.AutoMigrate(&db.Game{}))
	return dBOject
}

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

func TestGetGameByID_ReturnsNilForNonExistentGame(t *testing.T) {
	testDB := setupTestDBForGames(t)
	db.Db = testDB

	game, err := db.GetGameByID(1)
	require.NoError(t, err)
	assert.Nil(t, game)
}

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

func TestSearchGamesByName_ReturnsEmptyForNoMatches(t *testing.T) {
	testDB := setupTestDBForGames(t)
	db.Db = testDB

	err := db.PutInGame(1, "Test Game 1", "Test Data 1")
	require.NoError(t, err)

	games, err := db.SearchGamesByName("Nonexistent")
	require.NoError(t, err)
	assert.Empty(t, games)
}
