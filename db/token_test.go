package db_test

import (
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDBForToken sets up an in-memory SQLite database for testing purposes.
// It returns a pointer to the gorm.DB instance.
func setupTestDBForToken(t *testing.T) *gorm.DB {
	dBOject, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, dBOject.AutoMigrate(&db.Token{}))
	return dBOject
}

// TestGetTokenRecord_ReturnsToken tests the retrieval of a token record from the database.
func TestGetTokenRecord_ReturnsToken(t *testing.T) {
	testDB := setupTestDBForToken(t)
	db.Db = testDB

	token := &db.Token{AccessToken: "access_token", RefreshToken: "refresh_token", ExpiresAt: "expires_at"}
	err := db.UpsertTokenRecord(token)
	require.NoError(t, err)

	retrievedToken, err := db.GetTokenRecord()
	require.NoError(t, err)
	assert.NotNil(t, retrievedToken)
	assert.Equal(t, "access_token", retrievedToken.AccessToken)
	assert.Equal(t, "refresh_token", retrievedToken.RefreshToken)
	assert.Equal(t, "expires_at", retrievedToken.ExpiresAt)
}

// TestGetTokenRecord_ReturnsNilForNoToken tests that GetTokenRecord returns nil when no token is found.
func TestGetTokenRecord_ReturnsNilForNoToken(t *testing.T) {
	testDB := setupTestDBForToken(t)
	db.Db = testDB

	retrievedToken, err := db.GetTokenRecord()
	require.NoError(t, err)
	assert.Nil(t, retrievedToken)
}

// TestGetTokenRecord_ReturnsErrorForUninitializedDB tests that GetTokenRecord returns an error when the database is not initialized.
func TestGetTokenRecord_ReturnsErrorForUninitializedDB(t *testing.T) {
	db.Db = nil

	retrievedToken, err := db.GetTokenRecord()
	assert.Error(t, err)
	assert.Nil(t, retrievedToken)
}

// TestUpsertTokenRecord_InsertsNewToken tests the insertion of a new token record into the database.
func TestUpsertTokenRecord_InsertsNewToken(t *testing.T) {
	testDB := setupTestDBForToken(t)
	db.Db = testDB

	token := &db.Token{AccessToken: "access_token", RefreshToken: "refresh_token", ExpiresAt: "expires_at"}
	err := db.UpsertTokenRecord(token)
	require.NoError(t, err)

	var retrievedToken db.Token
	err = testDB.First(&retrievedToken, "1 = 1").Error
	require.NoError(t, err)
	assert.Equal(t, "access_token", retrievedToken.AccessToken)
	assert.Equal(t, "refresh_token", retrievedToken.RefreshToken)
	assert.Equal(t, "expires_at", retrievedToken.ExpiresAt)
}

// TestUpsertTokenRecord_UpdatesExistingToken tests the update of an existing token record in the database.
func TestUpsertTokenRecord_UpdatesExistingToken(t *testing.T) {
	testDB := setupTestDBForToken(t)
	db.Db = testDB

	token := &db.Token{AccessToken: "access_token", RefreshToken: "refresh_token", ExpiresAt: "expires_at"}
	err := db.UpsertTokenRecord(token)
	require.NoError(t, err)

	updatedToken := &db.Token{AccessToken: "new_access_token", RefreshToken: "new_refresh_token", ExpiresAt: "new_expires_at"}
	err = db.UpsertTokenRecord(updatedToken)
	require.NoError(t, err)

	var retrievedToken db.Token
	err = testDB.First(&retrievedToken, "1 = 1").Error
	require.NoError(t, err)
	assert.Equal(t, "new_access_token", retrievedToken.AccessToken)
	assert.Equal(t, "new_refresh_token", retrievedToken.RefreshToken)
	assert.Equal(t, "new_expires_at", retrievedToken.ExpiresAt)
}

// TestUpsertTokenRecord_ReturnsErrorForUninitializedDB tests that UpsertTokenRecord returns an error when the database is not initialized.
func TestUpsertTokenRecord_ReturnsErrorForUninitializedDB(t *testing.T) {
	db.Db = nil

	token := &db.Token{AccessToken: "access_token", RefreshToken: "refresh_token", ExpiresAt: "expires_at"}
	err := db.UpsertTokenRecord(token)
	assert.Error(t, err)
}
