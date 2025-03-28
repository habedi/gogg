// catalogue_test.go
package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/spf13/cobra"
)

func initTestDB(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	db.Path = filepath.Join(tmpDir, "games.db")
	if err := db.InitDB(); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	// Make sure to close the DB when the test is done.
	t.Cleanup(func() {
		if err := db.CloseDB(); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	})
}

// addTestGame adds a dummy game into the catalogue.
func addTestGame(t *testing.T, id int, title, data string) {
	t.Helper()
	if err := db.PutInGame(id, title, data); err != nil {
		t.Fatalf("failed to add game: %v", err)
	}
}

// captureCombinedOutput runs the given command while redirecting os.Stdout,
// and returns the combined output from both the command's output and os.Stdout.
func captureCombinedOutput(cmd *cobra.Command, args ...string) (string, error) {
	// Backup the original os.Stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w

	// Also capture cmd's output in a buffer.
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	// Execute the command.
	cmdErr := cmd.Execute()

	// Restore os.Stdout.
	w.Close()
	os.Stdout = oldStdout

	// Read what was written to the pipe.
	pipeOutput, _ := io.ReadAll(r)
	r.Close()

	// Combine the output.
	return buf.String() + string(pipeOutput), cmdErr
}

// TestListCmd tests the "catalogue list" command.
func TestListCmd(t *testing.T) {
	initTestDB(t)
	// Insert dummy games.
	dummyData := `{"dummy": "data"}`
	addTestGame(t, 1, "Test Game 1", dummyData)
	addTestGame(t, 2, "Test Game 2", dummyData)

	// Get the list command.
	listCommand := listCmd()
	output, err := captureCombinedOutput(listCommand)
	if err != nil {
		t.Errorf("list command failed: %v", err)
	}

	if !strings.Contains(output, "Test Game 1") || !strings.Contains(output, "Test Game 2") {
		t.Errorf("expected output to contain game titles, got: %s", output)
	}
}

// TestInfoCmd tests the "catalogue info" command.
func TestInfoCmd(t *testing.T) {
	initTestDB(t)
	// Insert a dummy game with valid JSON data.
	nested := map[string]interface{}{
		"description": "A cool game",
		"rating":      5,
	}
	nestedBytes, err := json.Marshal(nested)
	if err != nil {
		t.Fatalf("failed to marshal dummy nested data: %v", err)
	}
	addTestGame(t, 10, "Info Test Game", string(nestedBytes))

	// Create the info command.
	infoCommand := infoCmd()
	// Pass game ID "10" as argument.
	output, err := captureCombinedOutput(infoCommand, "10")
	if err != nil {
		t.Errorf("info command failed: %v", err)
	}
	if !strings.Contains(output, "cool game") {
		t.Errorf("expected output to contain 'cool game', got: %s", output)
	}
}

// TestSearchCmd tests the "catalogue search" command.
func TestSearchCmd(t *testing.T) {
	initTestDB(t)
	// Insert dummy games.
	dummyData := `{"dummy": "data"}`
	addTestGame(t, 20, "Awesome Game", dummyData)
	addTestGame(t, 21, "Not So Awesome", dummyData)

	// Test search by title (default).
	searchCommand := searchCmd()
	output, err := captureCombinedOutput(searchCommand, "Awesome")
	if err != nil {
		t.Errorf("search command failed: %v", err)
	}
	if !strings.Contains(output, "Awesome Game") {
		t.Errorf("expected output to contain 'Awesome Game', got: %s", output)
	}

	// Test search by ID using --id flag.
	// Insert a game with a known ID.
	addTestGame(t, 30, "ID Game", dummyData)
	searchCommand = searchCmd()
	// When searching by ID, the argument must be numeric.
	output, err = captureCombinedOutput(searchCommand, "30", "--id")
	if err != nil {
		t.Errorf("search command with --id failed: %v", err)
	}
	if !strings.Contains(output, "ID Game") {
		t.Errorf("expected output to contain 'ID Game', got: %s", output)
	}
}

// TestExportCmd tests the "catalogue export" command for JSON and CSV formats.
func TestExportCmd(t *testing.T) {
	initTestDB(t)
	// Insert a dummy game.
	dummyData := `{"dummy": "data"}`
	addTestGame(t, 40, "Export Test Game", dummyData)

	// Create a temporary directory to export files.
	tmpExportDir := t.TempDir()

	// Test JSON export.
	exportCommand := exportCmd()
	// Set the format flag to json.
	exportCommand.Flags().Set("format", "json")
	output, err := captureCombinedOutput(exportCommand, tmpExportDir)
	if err != nil {
		t.Errorf("export command (json) failed: %v", err)
	}
	// Check that the output mentions a file path.
	if !strings.Contains(output, tmpExportDir) {
		t.Errorf("expected output to mention export directory, got: %s", output)
	}
	// Verify the JSON file exists.
	files, err := os.ReadDir(tmpExportDir)
	if err != nil {
		t.Fatalf("failed to read export directory: %v", err)
	}
	var jsonFileFound bool
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			jsonFileFound = true
			// Optionally, read and verify file content.
			content, err := os.ReadFile(filepath.Join(tmpExportDir, f.Name()))
			if err != nil {
				t.Errorf("failed to read exported JSON file: %v", err)
			}
			if !strings.Contains(string(content), "Export Test Game") {
				t.Errorf("exported JSON file does not contain expected game title, got: %s", string(content))
			}
		}
	}
	if !jsonFileFound {
		t.Errorf("expected a JSON file to be exported in %s", tmpExportDir)
	}

	// Test CSV export.
	// Clear the export directory.
	os.RemoveAll(tmpExportDir)
	os.MkdirAll(tmpExportDir, 0o750)
	exportCommand = exportCmd()
	exportCommand.Flags().Set("format", "csv")
	_, err = captureCombinedOutput(exportCommand, tmpExportDir)
	if err != nil {
		t.Errorf("export command (csv) failed: %v", err)
	}
	files, err = os.ReadDir(tmpExportDir)
	if err != nil {
		t.Fatalf("failed to read export directory: %v", err)
	}
	var csvFileFound bool
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".csv" {
			csvFileFound = true
			content, err := os.ReadFile(filepath.Join(tmpExportDir, f.Name()))
			if err != nil {
				t.Errorf("failed to read exported CSV file: %v", err)
			}
			if !strings.Contains(string(content), "Export Test Game") {
				t.Errorf("exported CSV file does not contain expected game title, got: %s", string(content))
			}
		}
	}
	if !csvFileFound {
		t.Errorf("expected a CSV file to be exported in %s", tmpExportDir)
	}
}

// TestRefreshCmd tests the "catalogue refresh" command in the case where no token exists.
// Since refreshCmd calls client.RefreshToken (which in turn depends on a token in the DB),
// and we haven't inserted any token, we expect an error message.
func TestRefreshCmd(t *testing.T) {
	initTestDB(t)
	// Ensure there is no token record in the DB.
	refreshCommand := refreshCmd()
	output, err := captureCombinedOutput(refreshCommand)
	// We expect no error from the command execution itself,
	// but the output should contain an error message.
	if err != nil {
		t.Errorf("refresh command execution error: %v", err)
	}
	if !strings.Contains(output, "Error: Failed to refresh the access token") &&
		!strings.Contains(output, "Did you login?") {
		t.Errorf("expected refresh command to complain about missing token, got: %s", output)
	}
}

// Additional tests for the db package remain unchanged.

func TestInitDB(t *testing.T) {
	initTestDB(t)
}

func TestPutInGameAndGetGameByID(t *testing.T) {
	initTestDB(t)
	if err := db.PutInGame(1, "Test Game", "Some test data"); err != nil {
		t.Fatalf("failed to put game: %v", err)
	}
	game, err := db.GetGameByID(1)
	if err != nil {
		t.Fatalf("failed to get game by ID: %v", err)
	}
	if game == nil || game.Title != "Test Game" {
		t.Errorf("expected game title 'Test Game', got: %v", game)
	}
}

func TestGetCatalogueAndEmptyCatalogue(t *testing.T) {
	initTestDB(t)
	if err := db.PutInGame(1, "Game One", "Data One"); err != nil {
		t.Fatalf("failed to put game one: %v", err)
	}
	if err := db.PutInGame(2, "Game Two", "Data Two"); err != nil {
		t.Fatalf("failed to put game two: %v", err)
	}
	games, err := db.GetCatalogue()
	if err != nil {
		t.Fatalf("failed to get catalogue: %v", err)
	}
	if len(games) != 2 {
		t.Errorf("expected 2 games, got: %d", len(games))
	}
	if err := db.EmptyCatalogue(); err != nil {
		t.Fatalf("failed to empty catalogue: %v", err)
	}
	games, err = db.GetCatalogue()
	if err != nil {
		t.Fatalf("failed to get catalogue after emptying: %v", err)
	}
	if len(games) != 0 {
		t.Errorf("expected 0 games after emptying, got: %d", len(games))
	}
}

func TestSearchGamesByName(t *testing.T) {
	initTestDB(t)
	if err := db.PutInGame(1, "Alpha Game", "Data A"); err != nil {
		t.Fatalf("failed to put game: %v", err)
	}
	if err := db.PutInGame(2, "Beta Game", "Data B"); err != nil {
		t.Fatalf("failed to put game: %v", err)
	}
	if err := db.PutInGame(3, "Gamma", "Data C"); err != nil {
		t.Fatalf("failed to put game: %v", err)
	}
	games, err := db.SearchGamesByName("Game")
	if err != nil {
		t.Fatalf("failed to search games: %v", err)
	}
	if len(games) != 2 {
		t.Errorf("expected 2 games matching 'Game', got: %d", len(games))
	}
}

func TestTokenOperations(t *testing.T) {
	initTestDB(t)
	// Initially, token should not exist.
	token, err := db.GetTokenRecord()
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}
	if token != nil {
		t.Errorf("expected no token, got: %v", token)
	}
	// Insert token.
	newToken := &db.Token{
		AccessToken:  "access123456",
		RefreshToken: "refresh123456",
		ExpiresAt:    "2025-01-01",
	}
	if err := db.UpsertTokenRecord(newToken); err != nil {
		t.Fatalf("failed to upsert token: %v", err)
	}
	// Update token.
	newToken.AccessToken = "newaccess7890"
	newToken.RefreshToken = "newrefresh7890"
	newToken.ExpiresAt = "2030-12-31"
	if err := db.UpsertTokenRecord(newToken); err != nil {
		t.Fatalf("failed to update token: %v", err)
	}
}

func TestCloseDB(t *testing.T) {
	initTestDB(t)
	if err := db.CloseDB(); err != nil {
		t.Fatalf("failed to close DB: %v", err)
	}
}
