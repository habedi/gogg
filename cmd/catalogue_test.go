package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestMain sets up the database once for all tests in this package.
func TestMain(m *testing.M) {
	// Setup: Initialize the database once.
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

func addTestGame(t *testing.T, id int, title, data string) {
	t.Helper()
	if err := db.PutInGame(id, title, data); err != nil {
		t.Fatalf("failed to add game: %v", err)
	}
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
	dummyData := `{"dummy": "data"}`
	addTestGame(t, 1, "Test Game 1", dummyData)
	addTestGame(t, 2, "Test Game 2", dummyData)
	listCommand := listCmd()
	output, err := captureCombinedOutput(listCommand)
	require.NoError(t, err)
	assert.Contains(t, output, "Test Game 1")
	assert.Contains(t, output, "Test Game 2")
}

func TestInfoCmd(t *testing.T) {
	cleanDBTables(t)
	nested := map[string]interface{}{
		"description": "A cool game",
		"rating":      5,
	}
	nestedBytes, err := json.Marshal(nested)
	require.NoError(t, err)
	addTestGame(t, 10, "Info Test Game", string(nestedBytes))
	infoCommand := infoCmd()
	output, err := captureCombinedOutput(infoCommand, "10")
	require.NoError(t, err)
	assert.Contains(t, output, "cool game")
}

func TestSearchCmd(t *testing.T) {
	cleanDBTables(t)
	dummyData := `{"dummy": "data"}`
	addTestGame(t, 20, "Awesome Game", dummyData)
	addTestGame(t, 21, "Not So Awesome", dummyData)
	searchCommand := searchCmd()
	output, err := captureCombinedOutput(searchCommand, "Awesome")
	require.NoError(t, err)
	assert.Contains(t, output, "Awesome Game")
	assert.Contains(t, output, "Not So Awesome")

	addTestGame(t, 30, "ID Game", dummyData)
	searchCommand = searchCmd()
	output, err = captureCombinedOutput(searchCommand, "30", "--id")
	require.NoError(t, err)
	assert.Contains(t, output, "ID Game")
}

func TestExportCmd(t *testing.T) {
	cleanDBTables(t)
	dummyData := `{"dummy": "data"}`
	addTestGame(t, 40, "Export Test Game", dummyData)
	tmpExportDir := t.TempDir()
	exportCommand := exportCmd()
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
