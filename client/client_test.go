package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSanitizePath verifies that SanitizePath properly cleans up a string.
func TestSanitizePath(t *testing.T) {
	input := "Test® Game: (Awesome)"
	expected := "testgame-awesome"
	result := SanitizePath(input)
	if result != expected {
		t.Errorf("SanitizePath(%q) = %q, expected %q", input, result, expected)
	}
}

// TestParseGameData verifies that a valid JSON string is correctly parsed into a Game.
func TestParseGameData(t *testing.T) {
	jsonStr := `{
		"title": "Test Game",
		"downloads": [
			[
				"en",
				{"windows": [{"manualUrl": "/file.txt", "name": "File", "size": "123"}]}
			]
		],
		"extras": [],
		"dlcs": []
	}`
	game, err := ParseGameData(jsonStr)
	if err != nil {
		t.Fatalf("ParseGameData failed: %v", err)
	}
	if game.Title != "Test Game" {
		t.Errorf("Expected title 'Test Game', got %q", game.Title)
	}
	if len(game.Downloads) != 1 {
		t.Errorf("Expected 1 download, got %d", len(game.Downloads))
	}
	if game.Downloads[0].Language != "en" {
		t.Errorf("Expected download language 'en', got %q", game.Downloads[0].Language)
	}
}

// TestCreateRequest ensures that the created request has the proper Authorization header.
func TestCreateRequest(t *testing.T) {
	req, err := createRequest("GET", "http://example.com", "dummy-token")
	if err != nil {
		t.Fatalf("createRequest failed: %v", err)
	}
	if req.Header.Get("Authorization") != "Bearer dummy-token" {
		t.Errorf("Expected Authorization header 'Bearer dummy-token', got %q", req.Header.Get("Authorization"))
	}
}

// TestDownloadGameFiles simulates a file download via an HTTP test server.
func TestDownloadGameFiles(t *testing.T) {
	// File content to serve.
	fileContent := "dummy file content"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For HEAD request, return the Content-Length.
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "18") // len("dummy file content") = 18
			w.WriteHeader(http.StatusOK)
			return
		}
		// For GET request, write the file content.
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Length", "18")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, fileContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer ts.Close()

	// Prepare a dummy Game with one download entry.
	manualURL := ts.URL + "/testfile.txt"
	download := Downloadable{
		Language: "en",
		Platforms: Platform{
			Windows: []PlatformFile{
				{
					ManualURL: &manualURL,
					Name:      "testfile.txt",
					Size:      "18",
				},
			},
		},
	}
	game := Game{
		Title:     "Test Game",
		Downloads: []Downloadable{download},
		Extras:    []Extra{},
		DLCs:      []DLC{},
	}

	// Use a temporary directory for downloads.
	tmpDir := t.TempDir()

	err := DownloadGameFiles("dummy-token", game, tmpDir, "en", "windows", false, false, false, false, 1)
	if err != nil {
		t.Fatalf("DownloadGameFiles failed: %v", err)
	}

	// The file path is built as: downloadPath/SanitizePath(game.Title)/"windows"/basename(manualURL)
	sanitizedTitle := SanitizePath(game.Title)
	filePath := filepath.Join(tmpDir, sanitizedTitle, "windows", filepath.Base(manualURL))
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	if string(data) != fileContent {
		t.Errorf("Downloaded file content = %q, expected %q", string(data), fileContent)
	}
}

// TestFetchGameData simulates fetching game data from a test HTTP server.
func TestFetchGameData(t *testing.T) {
	// Dummy JSON representing a game.
	gameJSON := `{"title": "Dummy Game", "downloads": [], "extras": [], "dlcs": []}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, gameJSON)
	}))
	defer ts.Close()

	game, raw, err := FetchGameData("dummy-token", ts.URL)
	if err != nil {
		t.Fatalf("FetchGameData failed: %v", err)
	}
	if game.Title != "Dummy Game" {
		t.Errorf("Expected game title 'Dummy Game', got %q", game.Title)
	}
	// Remove any extra whitespace.
	if strings.TrimSpace(raw) != gameJSON {
		t.Errorf("Raw response %q does not match expected %q", raw, gameJSON)
	}
}

// TestFetchIdOfOwnedGames simulates fetching owned game IDs from a test HTTP server.
func TestFetchIdOfOwnedGames(t *testing.T) {
	ownedJSON := `{"owned": [1, 2, 3]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, ownedJSON)
	}))
	defer ts.Close()

	ids, err := FetchIdOfOwnedGames("dummy-token", ts.URL)
	if err != nil {
		t.Fatalf("FetchIdOfOwnedGames failed: %v", err)
	}
	expected := []int{1, 2, 3}
	if len(ids) != len(expected) {
		t.Fatalf("Expected %d ids, got %d", len(expected), len(ids))
	}
	for i, id := range expected {
		if ids[i] != id {
			t.Errorf("Expected id %d at index %d, got %d", id, i, ids[i])
		}
	}
}
