package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBPathSelection(t *testing.T) {
	originalPath := db.Path
	t.Cleanup(func() {
		db.Path = originalPath
	})

	t.Run("uses GOGG_HOME when set", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("GOGG_HOME", tempDir)
		t.Setenv("XDG_DATA_HOME", "should_be_ignored")

		db.ConfigurePath()

		expected := filepath.Join(tempDir, "games.db")
		assert.Equal(t, expected, db.Path)
	})

	t.Run("uses XDG_DATA_HOME when GOGG_HOME is not set", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("GOGG_HOME", "")
		t.Setenv("XDG_DATA_HOME", tempDir)

		db.ConfigurePath()

		expected := filepath.Join(tempDir, "gogg", "games.db")
		assert.Equal(t, expected, db.Path)
	})

	t.Run("uses default home directory as a fallback", func(t *testing.T) {
		t.Setenv("GOGG_HOME", "")
		t.Setenv("XDG_DATA_HOME", "")

		db.ConfigurePath()

		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		expected := filepath.Join(homeDir, ".gogg", "games.db")
		assert.Equal(t, expected, db.Path)
	})
}

func TestInitDB(t *testing.T) {
	tempDir := t.TempDir()
	db.Path = filepath.Join(tempDir, ".gogg", "games.db")

	err := db.InitDB()
	require.NoError(t, err, "InitDB should not return an error")

	_, statErr := os.Stat(db.Path)
	assert.NoError(t, statErr, "Database file should exist")

	err = db.CloseDB()
	assert.NoError(t, err, "CloseDB should not return an error")
}

func TestCloseDB(t *testing.T) {
	t.Run("it does nothing if DB is not initialized", func(t *testing.T) {
		db.Db = nil // Ensure DB is nil
		err := db.CloseDB()
		assert.NoError(t, err)
	})

	t.Run("it closes an open DB connection", func(t *testing.T) {
		db.Path = filepath.Join(t.TempDir(), "test.db")
		require.NoError(t, db.InitDB())

		err := db.CloseDB()
		assert.NoError(t, err, "CloseDB should not return an error")
	})
}
