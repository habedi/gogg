package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateRequest(t *testing.T) {
	req, err := createRequest("GET", "http://example.com/path", "mytoken")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	auth := req.Header.Get("Authorization")
	expected := "Bearer mytoken"
	if auth != expected {
		t.Errorf("Expected Authorization header %q, got %q", expected, auth)
	}
}

func TestParseRawDownloads(t *testing.T) {
	raw := [][]interface{}{
		{"en", map[string]interface{}{"windows": []interface{}{map[string]interface{}{"name": "setup.exe", "size": "1GB"}}}},
		{123, "invalid"},
		{"fr"},
	}
	downloads := parseRawDownloads(raw)
	if len(downloads) != 1 {
		t.Fatalf("Expected 1 valid download entry, got %d", len(downloads))
	}
	dl := downloads[0]
	assert.Equal(t, "en", dl.Language)
	assert.Len(t, dl.Platforms.Windows, 1)
	file := dl.Platforms.Windows[0]
	assert.Equal(t, "setup.exe", file.Name)
	assert.Equal(t, "1GB", file.Size)
}

func TestParseGameData(t *testing.T) {
	valid := `{"title":"Test","downloads":[],"extras":[],"dlcs":[]}`
	g, err := ParseGameData(valid)
	if err != nil {
		t.Fatalf("Unexpected error parsing valid JSON: %v", err)
	}
	if g.Title != "Test" {
		t.Errorf("Expected title Test, got %s", g.Title)
	}
	_, err = ParseGameData("invalid json")
	if err == nil {
		t.Error("Expected error parsing invalid JSON, got nil")
	}
}

func TestSanitizePath(t *testing.T) {
	cases := map[string]string{
		"My Game® (Test)™":  "my-game-test",
		"Spaces And:Colons": "spaces-andcolons",
		"UPPER_case":        "upper_case",
	}
	for input, expected := range cases {
		got := SanitizePath(input)
		if got != expected {
			t.Errorf("SanitizePath(%q) = %q; want %q", input, got, expected)
		}
	}
}

func TestEnsureDirExists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "subdir")
	err := ensureDirExists(path)
	assert.NoError(t, err)
	info, err := os.Stat(path)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())

	filePath := filepath.Join(tmp, "file.txt")
	os.WriteFile(filePath, []byte("data"), 0o644)
	err = ensureDirExists(filePath)
	if err == nil {
		t.Error("Expected error when path exists and is not a directory, got nil")
	}
}
