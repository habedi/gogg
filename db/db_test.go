package db

import (
	"path/filepath"
	"testing"
)

func setupTestDB(t *testing.T) func() {
	tempDir := t.TempDir()
	Path = filepath.Join(tempDir, "games.db")
	if err := InitDB(); err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	return func() {
		if err := CloseDB(); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	}
}

func TestInitDB(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	if Db == nil {
		t.Error("expected Db to be initialized, got nil")
	}
}

func TestPutInGameAndGetGameByID(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	id := 1
	title := "Test Game"
	data := "Some test data"
	if err := PutInGame(id, title, data); err != nil {
		t.Fatalf("PutInGame failed: %v", err)
	}

	game, err := GetGameByID(id)
	if err != nil {
		t.Fatalf("GetGameByID failed: %v", err)
	}
	if game == nil {
		t.Fatalf("expected game with id %d, got nil", id)
	}
	if game.Title != title || game.Data != data {
		t.Errorf("expected title %q and data %q, got title %q and data %q", title, data, game.Title, game.Data)
	}
}

func TestGetCatalogueAndEmptyCatalogue(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	gamesToInsert := []Game{
		{ID: 1, Title: "Game One", Data: "Data One"},
		{ID: 2, Title: "Game Two", Data: "Data Two"},
	}
	for _, game := range gamesToInsert {
		if err := PutInGame(game.ID, game.Title, game.Data); err != nil {
			t.Fatalf("failed to insert game: %v", err)
		}
	}

	games, err := GetCatalogue()
	if err != nil {
		t.Fatalf("GetCatalogue failed: %v", err)
	}
	if len(games) != len(gamesToInsert) {
		t.Errorf("expected %d games, got %d", len(gamesToInsert), len(games))
	}

	if err := EmptyCatalogue(); err != nil {
		t.Fatalf("EmptyCatalogue failed: %v", err)
	}
	games, err = GetCatalogue()
	if err != nil {
		t.Fatalf("GetCatalogue failed: %v", err)
	}
	if len(games) != 0 {
		t.Errorf("expected empty catalogue, got %d games", len(games))
	}
}

func TestSearchGamesByName(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	gamesToInsert := []Game{
		{ID: 1, Title: "Alpha Game", Data: "Data A"},
		{ID: 2, Title: "Beta Game", Data: "Data B"},
		{ID: 3, Title: "Gamma", Data: "Data C"},
	}
	for _, game := range gamesToInsert {
		if err := PutInGame(game.ID, game.Title, game.Data); err != nil {
			t.Fatalf("failed to insert game: %v", err)
		}
	}

	results, err := SearchGamesByName("Game")
	if err != nil {
		t.Fatalf("SearchGamesByName failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 games containing 'Game', got %d", len(results))
	}

	results, err = SearchGamesByName("Nonexistent")
	if err != nil {
		t.Fatalf("SearchGamesByName failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 games, got %d", len(results))
	}
}

func TestTokenOperations(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Initially, there should be no token.
	token, err := GetTokenRecord()
	if err != nil {
		t.Fatalf("GetTokenRecord failed: %v", err)
	}
	if token != nil {
		t.Errorf("expected no token record, got %+v", token)
	}

	// Insert a new token.
	newToken := &Token{
		AccessToken:  "access123456",
		RefreshToken: "refresh123456",
		ExpiresAt:    "2025-01-01",
	}
	if err := UpsertTokenRecord(newToken); err != nil {
		t.Fatalf("UpsertTokenRecord failed: %v", err)
	}

	token, err = GetTokenRecord()
	if err != nil {
		t.Fatalf("GetTokenRecord failed: %v", err)
	}
	if token == nil {
		t.Fatalf("expected token record, got nil")
	}
	if token.AccessToken != newToken.AccessToken || token.RefreshToken != newToken.RefreshToken || token.ExpiresAt != newToken.ExpiresAt {
		t.Errorf("expected token %+v, got %+v", newToken, token)
	}

	// Update the token record.
	updatedToken := &Token{
		AccessToken:  "newaccess7890",
		RefreshToken: "newrefresh7890",
		ExpiresAt:    "2030-12-31",
	}
	if err := UpsertTokenRecord(updatedToken); err != nil {
		t.Fatalf("UpsertTokenRecord update failed: %v", err)
	}

	token, err = GetTokenRecord()
	if err != nil {
		t.Fatalf("GetTokenRecord failed: %v", err)
	}
	if token.AccessToken != updatedToken.AccessToken || token.RefreshToken != updatedToken.RefreshToken || token.ExpiresAt != updatedToken.ExpiresAt {
		t.Errorf("expected updated token %+v, got %+v", updatedToken, token)
	}
}

func TestCloseDB(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	if err := CloseDB(); err != nil {
		t.Errorf("CloseDB failed: %v", err)
	}
}
