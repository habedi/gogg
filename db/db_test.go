package db_test

import (
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

// TestInitDB tests the initialization of the database.
// It sets up a temporary directory, initializes the database, and checks if the database file is created successfully.
func TestInitDB(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	db.Path = filepath.Join(tempDir, ".gogg/games.db")
	err := db.InitDB()
	assert.NoError(t, err, "InitDB should not return an error")

	// Check if the database file was created
	_, statErr := os.Stat(db.Path)
	assert.NoError(t, statErr, "Database file should exist")

	// Close the database to release the file handle
	closeErr := db.CloseDB()
	assert.NoError(t, closeErr, "CloseDB should not return an error")
}

// TestCloseDB tests the closing of the database connection.
// It ensures that the CloseDB function does not return an error.
func TestCloseDB(t *testing.T) {
	err := db.CloseDB()
	assert.NoError(t, err, "CloseDB should not return an error")
}
