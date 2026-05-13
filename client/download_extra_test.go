package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveNext and canonicalizeURL helpers -----------------------------------

func TestResolveNext_InvalidBaseURL(t *testing.T) {
	// null byte makes url.Parse(baseURL) fail → return next as-is
	result := resolveNext("\x00bad", "/relative")
	assert.Equal(t, "/relative", result)
}

func TestResolveNext_InvalidNextURL(t *testing.T) {
	// \x00 is not an absolute URL (no scheme), so first if is skipped,
	// then url.Parse(next) fails → return next as-is
	result := resolveNext("http://example.com", "\x00bad")
	assert.Equal(t, "\x00bad", result)
}

func TestCanonicalizeURL_InvalidURL(t *testing.T) {
	// null byte causes url.Parse to fail → original string is returned
	result := canonicalizeURL("\x00bad")
	assert.Equal(t, "\x00bad", result)
}

func TestCanonicalizeURL_QueryTrailingAmpersand(t *testing.T) {
	result := canonicalizeURL("http://example.com/games?page=1&")
	assert.Equal(t, "http://example.com/games?page=1", result)
}

// Download paths ------------------------------------------------------------

func newSimpleServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.Header.Get("Authorization") == "" {
				// findFileLocation GET — return 200 with no body to signal no redirect
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusOK)
			w.Write(body)
		case http.MethodHead:
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusOK)
		}
	}))
}

func gameWithURL(title, rawURL string) Game {
	return Game{
		Title: title,
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &rawURL, Name: "setup.exe"}}},
		}},
	}
}

func TestDownloadGameFiles_ResumePartFileAlreadyComplete(t *testing.T) {
	// .part file already has all the bytes — HEAD returns same Content-Length.
	// DownloadGameFiles must rename it and return nil without making a GET.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(http.StatusOK)
			return
		}
		// GET for findFileLocation — 200, no redirect
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/files/setup.exe"
	game := gameWithURL("resumegame", rawURL)

	// Pre-create the .part file fully populated.
	gameDir := filepath.Join(tmp, "resumegame", "windows")
	require.NoError(t, os.MkdirAll(gameDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gameDir, "setup.exe.part"), []byte("hello"), 0o644))

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		"en", "windows", false, false, true, false, false, false, 1, io.Discard)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(gameDir, "setup.exe"))
	assert.NoFileExists(t, filepath.Join(gameDir, "setup.exe.part"))
}

func TestDownloadGameFiles_RommLayout(t *testing.T) {
	// rommLayout=true puts files under platform/game/ instead of game/platform/
	server := newSimpleServer(t, []byte("x"))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/files/game.exe"
	game := gameWithURL("TestGame", rawURL)
	game.Downloads[0].Platforms.Windows[0].Name = "game.exe"

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		"en", "windows", false, false, false, false, false, true, 1, io.Discard)
	require.NoError(t, err)

	// RomM layout: downloadPath/platform/game/filename
	assert.FileExists(t, filepath.Join(tmp, "windows", "testgame", "game.exe"))
}

func TestDownloadGameFiles_FlattenFlag(t *testing.T) {
	// flatten=true omits the platform subdir — file lands directly under game dir.
	server := newSimpleServer(t, []byte("x"))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/files/setup.exe"
	game := gameWithURL("flatgame", rawURL)

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		"en", "windows", false, false, false, true, false, false, 1, io.Discard)
	require.NoError(t, err)

	// With flatten the platform subdir is omitted.
	assert.FileExists(t, filepath.Join(tmp, "flatgame", "setup.exe"))
	assert.NoDirExists(t, filepath.Join(tmp, "flatgame", "windows"))
}

func TestDownloadGameFiles_RedirectURL(t *testing.T) {
	// Server returns 302 on the manual URL; the actual file is at the redirect target.
	// After redirect the filename is taken from the new URL path.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			// findFileLocation receives this 302 and extracts the Location.
			http.Redirect(w, r, "http://"+r.Host+"/files/final.exe", http.StatusFound)
		case "/files/final.exe":
			if r.Method == http.MethodHead {
				w.Header().Set("Content-Length", "1")
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("x"))
		}
	}))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/redirect"
	game := gameWithURL("redgame", rawURL)
	game.Downloads[0].Platforms.Windows[0].Name = "original_name.exe"

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		"en", "windows", false, false, false, false, false, false, 1, io.Discard)
	require.NoError(t, err)

	// Filename should be replaced with the one from the redirect URL.
	gameDir := filepath.Join(tmp, "redgame", "windows")
	assert.FileExists(t, filepath.Join(gameDir, "final.exe"))
}

func TestDownloadGameFiles_RedirectMissingLocation(t *testing.T) {
	// Server returns 301 with no Location header — findFileLocation returns an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMovedPermanently) // 301 with no Location
	}))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/files/setup.exe"
	game := gameWithURL("badredirect", rawURL)

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		"en", "windows", false, false, false, false, false, false, 1, io.Discard)
	assert.Error(t, err)
}

func TestDownloadGameFiles_ExtrasFlag(t *testing.T) {
	// extrasFlag=true enqueues extras alongside the main download.
	server := newSimpleServer(t, []byte("x"))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/files/setup.exe"
	extraURL := server.URL + "/extras/soundtrack.zip"
	game := Game{
		Title: "extragame",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &rawURL, Name: "setup.exe"}}},
		}},
		// Real GOG extra names are display strings without extension; enqueueExtras
		// appends the extension from the ManualURL.
		Extras: []Extra{{Name: "Game Soundtrack", ManualURL: extraURL, Size: "1 MB"}},
	}

	err := DownloadGameFiles(context.Background(), "token", game, tmp,
		"en", "windows", true, false, false, false, false, false, 1, io.Discard)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(tmp, "extragame", "windows", "setup.exe"))
	// enqueueExtras: SanitizePath("Game Soundtrack") = "game-soundtrack", then appends ".zip" from URL
	assert.FileExists(t, filepath.Join(tmp, "extragame", "extras", "game-soundtrack.zip"))
}
