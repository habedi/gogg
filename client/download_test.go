package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// errWriter returns an error on every Write, used to exercise the writeProgress error branch.
type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) { return 0, errors.New("write failed") }

func TestWriteProgress_WriterError(t *testing.T) {
	pr := &progressReader{writer: errWriter{}}
	pr.writeProgress([]byte("data")) // must not panic; error is only logged
}

func TestSanitizePath_TruncatesAt200Chars(t *testing.T) {
	long := strings.Repeat("a", 300)
	result := SanitizePath(long)
	assert.LessOrEqual(t, len(result), 200)
	assert.NotEmpty(t, result)
}

func TestDownloadGameFiles_BadDownloadPath(t *testing.T) {
	tmp := t.TempDir()
	// Place a file where a directory is required so MkdirAll fails.
	blockingFile := filepath.Join(tmp, "block")
	require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o644))
	badPath := filepath.Join(blockingFile, "sub")

	err := DownloadGameFiles(context.Background(), "token", Game{}, badPath,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
	assert.Error(t, err)
}

func TestDownloadGameFiles_EmptyGame(t *testing.T) {
	// An empty Game with no downloads produces no tasks; the function succeeds
	// and writes a metadata.json file.
	tmp := t.TempDir()
	err := DownloadGameFiles(context.Background(), "token", Game{Title: "mygame"}, tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
	require.NoError(t, err)
}

func TestDownloadGameFiles_EnqueueError(t *testing.T) {
	// A pre-cancelled context causes enqueueGameFiles to return context.Canceled,
	// which propagates as the function's return value.
	tmp := t.TempDir()
	url := "/files/setup.exe"
	game := Game{Downloads: []Downloadable{{
		Language:  "en",
		Platforms: Platform{Windows: []PlatformFile{{ManualURL: &url, Name: "setup.exe"}}},
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := DownloadGameFiles(ctx, "token", game, tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDownloadGameFiles_HTTP403OnHEAD(t *testing.T) {
	// Server returns 403 on HEAD; the task must be skipped (nil error) and
	// the empty file must be removed. DownloadGameFiles returns no error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/dlc/setup.exe"
	game := Game{
		Title: "mygame",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &rawURL, Name: "setup.exe"}}},
		}},
	}

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
	require.NoError(t, err)

	// The empty partial file must have been cleaned up.
	entries, _ := os.ReadDir(filepath.Join(tmp, "mygame", "windows"))
	for _, e := range entries {
		assert.NotEqual(t, "setup.exe", e.Name(), "partial file should have been removed")
	}
}

func TestDownloadGameFiles_HTTP403OnGET(t *testing.T) {
	// Server returns 200 on HEAD but 403 on GET; same skip-and-no-error behaviour.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "1024")
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/dlc/setup.exe"
	game := Game{
		Title: "mygame",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &rawURL, Name: "setup.exe"}}},
		}},
	}

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
	require.NoError(t, err)
}

func TestDownloadGameFiles_PartFileRenamedOnSuccess(t *testing.T) {
	// Server responds with a complete 1-byte body. After download the final file
	// must exist and the .part file must be gone.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("x"))
	}))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/files/setup.exe"
	game := Game{
		Title: "mygame",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &rawURL, Name: "setup.exe"}}},
		}},
	}

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
	require.NoError(t, err)

	gameDir := filepath.Join(tmp, "mygame", "windows")
	assert.FileExists(t, filepath.Join(gameDir, "setup.exe"), "final file should exist")
	assert.NoFileExists(t, filepath.Join(gameDir, "setup.exe.part"), ".part file should be removed after success")
}

func TestDownloadGameFiles_PartFileRemovedOnError(t *testing.T) {
	// Server returns 500 after the file has been opened; the .part file must be
	// cleaned up when resume is not requested.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/files/setup.exe"
	game := Game{
		Title: "mygame",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &rawURL, Name: "setup.exe"}}},
		}},
	}

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
	require.Error(t, err)

	gameDir := filepath.Join(tmp, "mygame", "windows")
	assert.NoFileExists(t, filepath.Join(gameDir, "setup.exe.part"), ".part file should be removed on non-resume error")
}

func TestDownloadGameFiles_DownloadTaskError(t *testing.T) {
	// A null-byte in the URL is invalid; http.NewRequestWithContext fails inside
	// findFileLocation, so pool.Run collects the download error and the function
	// returns a "download tasks failed" error.
	tmp := t.TempDir()
	badURL := "\x00invalid"
	game := Game{
		Title: "mygame",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &badURL, Name: "setup.exe"}}},
		}},
	}

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download tasks failed")
}
