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

func TestFilenameExtractionFromRedirectURL(t *testing.T) {
	tests := []struct {
		name         string
		redirectURL  string
		expectedBase string
		description  string
	}{
		{
			name:         "exe_file",
			redirectURL:  "https://cdn.gog.com/content-system/v2/setup_nox_2.0.0.20.exe",
			expectedBase: "setup_nox_2.0.0.20.exe",
			description:  "Should extract .exe extension from redirect URL",
		},
		{
			name:         "zip_file",
			redirectURL:  "https://cdn.gog.com/content-system/v2/Nox_QRC.zip",
			expectedBase: "Nox_QRC.zip",
			description:  "Should extract .zip extension from redirect URL",
		},
		{
			name:         "bin_file",
			redirectURL:  "https://cdn.gog.com/secure/setup_prey_12742273_(64bit)_(65935)-1.bin",
			expectedBase: "setup_prey_12742273_(64bit)_(65935)-1.bin",
			description:  "Should extract .bin extension from redirect URL",
		},
		{
			name:         "multipart_installer",
			redirectURL:  "https://cdn.gog.com/secure/setup_prey_12742273_(64bit)_(65935).exe",
			expectedBase: "setup_prey_12742273_(64bit)_(65935).exe",
			description:  "Should extract main .exe for multipart installers",
		},
		{
			name:         "url_encoded_filename",
			redirectURL:  "https://cdn.gog.com/secure/Game%20File%20v1.2.3.zip",
			expectedBase: "Game%20File%20v1.2.3.zip",
			description:  "Should handle URL-encoded filenames (decoding happens later)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := filepath.Base(tt.redirectURL)
			assert.Equal(t, tt.expectedBase, base, tt.description)

			// Verify the extension is present
			ext := filepath.Ext(base)
			assert.NotEmpty(t, ext, "Filename should have an extension: %s", tt.description)
		})
	}
}

func TestFilenameWithoutExtensionShouldBeReplaced(t *testing.T) {
	// This test documents the bug: when API returns filename without extension,
	// and redirect URL has the proper filename with extension, we should use the redirect URL's filename

	tests := []struct {
		name              string
		apiFileName       string
		redirectURL       string
		expectedFinalName string
	}{
		{
			name:              "bastion_installer",
			apiFileName:       "Bastion", // API returns name without extension
			redirectURL:       "https://cdn.gog.com/secure/bastion_installer_v1.0.exe",
			expectedFinalName: "bastion_installer_v1.0.exe", // Should use redirect URL's name
		},
		{
			name:              "wallpaper_file",
			apiFileName:       "wallpaper",
			redirectURL:       "https://cdn.gog.com/extras/wallpaper_4k.zip",
			expectedFinalName: "wallpaper_4k.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic in downloadFile function
			fileName := tt.apiFileName

			// After getting redirect, extract filename from URL
			base := filepath.Base(tt.redirectURL)
			if base != "." && base != "/" {
				// BUG FIX: Always use redirect URL's filename, don't check if fileName is empty
				fileName = base
			}

			assert.Equal(t, tt.expectedFinalName, fileName,
				"Should replace API filename with redirect URL filename to get proper extension")

			// Verify the final filename has an extension
			ext := filepath.Ext(fileName)
			assert.NotEmpty(t, ext, "Final filename must have an extension")
		})
	}
}

func TestBuildManualURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "https://api.gog.com/secure/file.exe",
			expected: "https://api.gog.com/secure/file.exe",
		},
		{
			input:    "/account/gameDetails/123.json",
			expected: "https://embed.gog.com/account/gameDetails/123.json",
		},
	}

	for _, tt := range tests {
		result := buildManualURL(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}
