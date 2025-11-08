package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestMain sets up the database once for all tests in this package.
func TestMain(m *testing.M) {
	// Setup: Initialize the database at once.
	tmpDir, err := os.MkdirTemp("", "gogg-cmd-test-")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create temp dir for testing")
	}
	db.Path = filepath.Join(tmpDir, "games.db")
	if err := db.InitDB(); err != nil {
		log.Fatal().Err(err).Msg("Failed to init db for testing")
	}

	// Run all tests in the package.
	exitCode := m.Run()

	// Teardown: Clean up resources after all tests are done.
	if err := db.CloseDB(); err != nil {
		log.Error().Err(err).Msg("Failed to close db after testing")
	}
	os.RemoveAll(tmpDir)

	os.Exit(exitCode)
}

// cleanDBTables ensures test isolation by clearing tables before each test.
func cleanDBTables(t *testing.T) {
	t.Helper()
	err := db.Db.Exec("DELETE FROM games").Error
	require.NoError(t, err)
	err = db.Db.Exec("DELETE FROM tokens").Error
	require.NoError(t, err)
}

type mockTokenStorer struct {
	getTokenErr error
}

func (m *mockTokenStorer) GetTokenRecord() (*db.Token, error) {
	if m.getTokenErr != nil {
		return nil, m.getTokenErr
	}
	return &db.Token{RefreshToken: "valid-refresh-token"}, nil
}
func (m *mockTokenStorer) UpsertTokenRecord(token *db.Token) error { return nil }

type mockTokenRefresher struct{}

func (m *mockTokenRefresher) PerformTokenRefresh(refreshToken string) (string, string, int64, error) {
	return "new-access-token", "new-refresh-token", 3600, nil
}

func addTestGame(t *testing.T, repo db.GameRepository, id int, title, data string) {
	t.Helper()
	require.NoError(t, repo.Put(context.Background(), db.Game{ID: id, Title: title, Data: data}))
}

func captureCombinedOutput(cmd *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return buf.String(), err
}

func TestListCmd(t *testing.T) {
	cleanDBTables(t)
	repo := db.NewGameRepository(db.GetDB())
	dummyData := `{"dummy": "data"}`
	addTestGame(t, repo, 1, "Test Game 1", dummyData)
	addTestGame(t, repo, 2, "Test Game 2", dummyData)
	listCommand := listCmd(repo)
	output, err := captureCombinedOutput(listCommand)
	require.NoError(t, err)
	assert.Contains(t, output, "Test Game 1")
	assert.Contains(t, output, "Test Game 2")
}

func TestInfoCmd(t *testing.T) {
	cleanDBTables(t)
	repo := db.NewGameRepository(db.GetDB())
	nested := map[string]interface{}{
		"description": "A cool game",
		"rating":      5,
	}
	nestedBytes, err := json.Marshal(nested)
	require.NoError(t, err)
	addTestGame(t, repo, 10, "Info Test Game", string(nestedBytes))
	infoCommand := infoCmd(repo)
	output, err := captureCombinedOutput(infoCommand, "10")
	require.NoError(t, err)
	assert.Contains(t, output, "cool game")
}

func TestSearchCmd(t *testing.T) {
	cleanDBTables(t)
	repo := db.NewGameRepository(db.GetDB())
	dummyData := `{"dummy": "data"}`
	addTestGame(t, repo, 20, "Awesome Game", dummyData)
	addTestGame(t, repo, 21, "Not So Awesome", dummyData)
	searchCommand := searchCmd(repo)
	output, err := captureCombinedOutput(searchCommand, "Awesome")
	require.NoError(t, err)
	assert.Contains(t, output, "Awesome Game")
	assert.Contains(t, output, "Not So Awesome")

	addTestGame(t, repo, 30, "ID Game", dummyData)
	searchCommand = searchCmd(repo)
	output, err = captureCombinedOutput(searchCommand, "30", "--id")
	require.NoError(t, err)
	assert.Contains(t, output, "ID Game")
}

func TestExportCmd(t *testing.T) {
	cleanDBTables(t)
	repo := db.NewGameRepository(db.GetDB())
	dummyData := `{"dummy": "data"}`
	addTestGame(t, repo, 40, "Export Test Game", dummyData)
	tmpExportDir := t.TempDir()
	exportCommand := exportCmd(repo)
	exportCommand.Flags().Set("format", "json")
	output, err := captureCombinedOutput(exportCommand, tmpExportDir)
	require.NoError(t, err)
	assert.Contains(t, output, tmpExportDir)
	// ... rest of assertions
}

func TestRefreshCmd(t *testing.T) {
	cleanDBTables(t)
	storer := &mockTokenStorer{getTokenErr: errors.New("mock db error")}
	refresher := &mockTokenRefresher{}
	authService := auth.NewService(storer, refresher)

	refreshCommand := refreshCmd(authService)
	output, err := captureCombinedOutput(refreshCommand)
	require.NoError(t, err) // The command itself should not error, just print an error message

	expectedErrorMsg := "Error: Failed to refresh catalogue. Please check the logs for details."
	assert.Contains(t, output, expectedErrorMsg)
}

func TestCatalogueCliErr_NotFound(t *testing.T) {
	cleanDBTables(t)
	repo := db.NewGameRepository(db.GetDB())
	infoCommand := infoCmd(repo)
	output, err := captureCombinedOutput(infoCommand, "99999")
	require.NoError(t, err)
	assert.Contains(t, output, "Game not found")
}

func TestCatalogueCliErr_InvalidExportFormat(t *testing.T) {
	cleanDBTables(t)
	repo := db.NewGameRepository(db.GetDB())
	addTestGame(t, repo, 50, "Export Err Game", "{}")
	cmd := exportCmd(repo)
	cmd.Flags().Set("format", "xml")
	output, err := captureCombinedOutput(cmd, t.TempDir())
	require.NoError(t, err)
	assert.Contains(t, output, "Invalid export format")
}
