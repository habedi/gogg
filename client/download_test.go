package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGameDataFromDownload(t *testing.T) {
	validJSON := `{"title":"Test","downloads":[],"extras":[],"dlcs":[]}`
	g, err := ParseGameData(validJSON)
	require.NoError(t, err)
	assert.Equal(t, "Test", g.Title)

	_, err = ParseGameData("invalid json")
	assert.Error(t, err)
}

func TestDownloadSanitizePath(t *testing.T) {
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

func TestDownloadEnsureDirExists(t *testing.T) {
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
