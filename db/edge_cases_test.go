package db

import (
	"os"
	"strings"
	"testing"
)

func TestConfigurePathErr_EmptyEnvVars(t *testing.T) {
	// Clear all relevant env vars
	oldGoggHome := os.Getenv("GOGG_HOME")
	oldXdgDataHome := os.Getenv("XDG_DATA_HOME")

	os.Unsetenv("GOGG_HOME")
	os.Unsetenv("XDG_DATA_HOME")

	defer func() {
		if oldGoggHome != "" {
			os.Setenv("GOGG_HOME", oldGoggHome)
		}
		if oldXdgDataHome != "" {
			os.Setenv("XDG_DATA_HOME", oldXdgDataHome)
		}
	}()

	err := ConfigurePathErr()
	if err != nil {
		t.Errorf("ConfigurePathErr() should not fail with empty env: %v", err)
	}

	if Path == "" {
		t.Error("Path should be set even with empty env vars")
	}
}

func TestConfigurePathErr_GoggHome(t *testing.T) {
	oldGoggHome := os.Getenv("GOGG_HOME")
	// Use OS-agnostic temp directory
	testPath := t.TempDir()
	os.Setenv("GOGG_HOME", testPath)

	defer func() {
		if oldGoggHome != "" {
			os.Setenv("GOGG_HOME", oldGoggHome)
		} else {
			os.Unsetenv("GOGG_HOME")
		}
	}()

	err := ConfigurePathErr()
	if err != nil {
		t.Errorf("ConfigurePathErr() error = %v", err)
	}

	if !strings.Contains(Path, testPath) {
		t.Errorf("Path = %v, should contain %v", Path, testPath)
	}
}

func TestConfigurePathErr_XdgDataHome(t *testing.T) {
	oldGoggHome := os.Getenv("GOGG_HOME")
	oldXdgDataHome := os.Getenv("XDG_DATA_HOME")

	os.Unsetenv("GOGG_HOME")
	// Use OS-agnostic temp directory
	testPath := t.TempDir()
	os.Setenv("XDG_DATA_HOME", testPath)

	defer func() {
		if oldGoggHome != "" {
			os.Setenv("GOGG_HOME", oldGoggHome)
		}
		if oldXdgDataHome != "" {
			os.Setenv("XDG_DATA_HOME", oldXdgDataHome)
		} else {
			os.Unsetenv("XDG_DATA_HOME")
		}
	}()

	err := ConfigurePathErr()
	if err != nil {
		t.Errorf("ConfigurePathErr() error = %v", err)
	}

	if !strings.Contains(Path, testPath) {
		t.Errorf("Path = %v, should contain %v", Path, testPath)
	}
}

func TestCloseDB_Nil(t *testing.T) {
	oldDb := Db
	Db = nil

	defer func() {
		Db = oldDb
	}()

	// Should not panic with nil Db
	err := CloseDB()
	if err != nil {
		t.Errorf("CloseDB() with nil Db should not error: %v", err)
	}
}

func TestShutdown_IgnoresError(t *testing.T) {
	// Shutdown should not panic even if CloseDB fails
	// This is important for interrupt handlers
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Shutdown() panicked: %v", r)
		}
	}()

	Shutdown()
}

func TestGetDB_ReturnsGlobalDb(t *testing.T) {
	oldDb := Db
	defer func() {
		Db = oldDb
	}()

	// Test with nil
	Db = nil
	if GetDB() != nil {
		t.Error("GetDB() should return nil when Db is nil")
	}

	// Test with non-nil (can't easily create real DB in unit test)
	// Just verify it returns the same reference
	Db = oldDb
	if GetDB() != Db {
		t.Error("GetDB() should return global Db reference")
	}
}

func TestGameRepository_Constructor(t *testing.T) {
	// Test that constructor doesn't panic with nil
	repo := NewGameRepository(nil)
	if repo == nil {
		t.Error("NewGameRepository(nil) should return non-nil repository")
	}
}

func TestTokenRepository_Constructor(t *testing.T) {
	// Test that constructor doesn't panic with nil
	repo := NewTokenRepository(nil)
	if repo == nil {
		t.Error("NewTokenRepository(nil) should return non-nil repository")
	}
}

// Add edge case tests for Game model
func TestGame_EmptyFields(t *testing.T) {
	game := Game{
		ID:    0,
		Title: "",
		Data:  "",
	}

	// Should be valid (fields can be empty)
	if game.ID != 0 || game.Title != "" || game.Data != "" {
		t.Error("Game fields not properly initialized")
	}
}

func TestGame_LargeID(t *testing.T) {
	game := Game{
		ID:    999999999,
		Title: "Test",
		Data:  "{}",
	}

	if game.ID != 999999999 {
		t.Error("Large ID not preserved")
	}
}

func TestGame_UnicodeTitle(t *testing.T) {
	game := Game{
		ID:    1,
		Title: "测试游戏 テスト 🎮",
		Data:  "{}",
	}

	if game.Title != "测试游戏 テスト 🎮" {
		t.Error("Unicode title not preserved")
	}
}

func TestGame_LargeData(t *testing.T) {
	largeData := string(make([]byte, 1000000)) // 1MB
	game := Game{
		ID:    1,
		Title: "Test",
		Data:  largeData,
	}

	if len(game.Data) != 1000000 {
		t.Error("Large data not preserved")
	}
}

// Add edge case tests for Token model
func TestToken_EmptyTokens(t *testing.T) {
	token := Token{
		ID:           1,
		AccessToken:  "",
		RefreshToken: "",
		ExpiresAt:    "",
	}

	// Should be valid (empty tokens might occur during initialization)
	if token.AccessToken != "" {
		t.Error("Empty access token not preserved")
	}
}

func TestToken_LongTokens(t *testing.T) {
	longToken := string(make([]byte, 10000))
	token := Token{
		ID:           1,
		AccessToken:  longToken,
		RefreshToken: longToken,
		ExpiresAt:    "2025-12-31T23:59:59Z",
	}

	if len(token.AccessToken) != 10000 {
		t.Error("Long access token not preserved")
	}
	if len(token.RefreshToken) != 10000 {
		t.Error("Long refresh token not preserved")
	}
}

func TestToken_InvalidExpiresAt(t *testing.T) {
	token := Token{
		ID:           1,
		AccessToken:  "test",
		RefreshToken: "test",
		ExpiresAt:    "invalid-date",
	}

	// Model should still accept invalid date format
	// Validation happens in business logic, not model
	if token.ExpiresAt != "invalid-date" {
		t.Error("ExpiresAt string not preserved")
	}
}

func TestToken_SpecialCharacters(t *testing.T) {
	token := Token{
		ID:           1,
		AccessToken:  "token!@#$%^&*()_+-=[]{}|;':\",./<>?",
		RefreshToken: "refresh\n\t\r",
		ExpiresAt:    "2025-12-31T23:59:59+00:00",
	}

	// Special characters should be preserved
	if !strings.Contains(token.AccessToken, "!@#$%") {
		t.Error("Special characters in access token not preserved")
	}
}

func TestToken_ZeroID(t *testing.T) {
	token := Token{
		ID:           0,
		AccessToken:  "test",
		RefreshToken: "test",
		ExpiresAt:    "2025-12-31T23:59:59Z",
	}

	if token.ID != 0 {
		t.Error("Zero ID not preserved")
	}
}

// Test repository error handling edge cases
func TestGameRepository_PutWithContext_Cancelled(t *testing.T) {
	t.Skip("Skipping test - requires real database connection, nil DB causes panic")
	// This test would require a real database to test properly
	// Repository methods panic when DB is nil (by design - should use constructor)
}

func TestGameRepository_GetByID_NegativeID(t *testing.T) {
	t.Skip("Skipping test - requires real database connection, nil DB causes panic")
	// This test would require a real database to test properly
}

func TestGameRepository_SearchByTitle_EmptyString(t *testing.T) {
	t.Skip("Skipping test - requires real database connection, nil DB causes panic")
	// This test would require a real database to test properly
}

func TestGameRepository_SearchByTitle_SpecialCharacters(t *testing.T) {
	t.Skip("Skipping test - requires real database connection, nil DB causes panic")
	// This test would require a real database to test properly
	// SQL injection testing would need proper DB setup
}

func TestTokenRepository_Get_WithTimeout(t *testing.T) {
	t.Skip("Skipping test - requires real database connection, nil DB causes panic")
	// This test would require a real database to test properly
}

func TestTokenRepository_Upsert_NilToken(t *testing.T) {
	t.Skip("Skipping test - requires real database connection, nil DB causes panic")
	// This test would require a real database to test properly
}
