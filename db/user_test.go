package db_test

import (
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func setupTestDBForUser(t *testing.T) *gorm.DB {
	dBOject, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, dBOject.AutoMigrate(&db.User{}))
	return dBOject
}

func TestGetUserData_ReturnsUser(t *testing.T) {
	testDB := setupTestDBForUser(t)
	db.Db = testDB

	user := &db.User{Username: "testuser", Password: "password"}
	err := db.UpsertUserData(user)
	require.NoError(t, err)

	retrievedUser, err := db.GetUserData()
	require.NoError(t, err)
	assert.NotNil(t, retrievedUser)
	assert.Equal(t, "testuser", retrievedUser.Username)
	assert.Equal(t, "password", retrievedUser.Password)
}

func TestGetUserData_ReturnsNilForNoUser(t *testing.T) {
	testDB := setupTestDBForUser(t)
	db.Db = testDB

	retrievedUser, err := db.GetUserData()
	require.NoError(t, err)
	assert.Nil(t, retrievedUser)
}

func TestGetUserData_ReturnsErrorForUninitializedDB(t *testing.T) {
	db.Db = nil

	retrievedUser, err := db.GetUserData()
	assert.Error(t, err)
	assert.Nil(t, retrievedUser)
}

func TestUpsertUserData_InsertsNewUser(t *testing.T) {
	testDB := setupTestDBForUser(t)
	db.Db = testDB

	user := &db.User{Username: "testuser", Password: "password"}
	err := db.UpsertUserData(user)
	require.NoError(t, err)

	var retrievedUser db.User
	err = testDB.First(&retrievedUser, "1 = 1").Error
	require.NoError(t, err)
	assert.Equal(t, "testuser", retrievedUser.Username)
	assert.Equal(t, "password", retrievedUser.Password)
}

func TestUpsertUserData_UpdatesExistingUser(t *testing.T) {
	testDB := setupTestDBForUser(t)
	db.Db = testDB

	user := &db.User{Username: "testuser", Password: "password"}
	err := db.UpsertUserData(user)
	require.NoError(t, err)

	updatedUser := &db.User{Username: "testuser", Password: "newpassword"}
	err = db.UpsertUserData(updatedUser)
	require.NoError(t, err)

	var retrievedUser db.User
	err = testDB.First(&retrievedUser, "1 = 1").Error
	require.NoError(t, err)
	assert.Equal(t, "testuser", retrievedUser.Username)
	assert.Equal(t, "newpassword", retrievedUser.Password)
}

func TestUpsertUserData_ReturnsErrorForUninitializedDB(t *testing.T) {
	db.Db = nil

	user := &db.User{Username: "testuser", Password: "password"}
	err := db.UpsertUserData(user)
	assert.Error(t, err)
}
