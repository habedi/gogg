package client

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
		DownloadOptions{Language: "en", Platform: "windows", Resume: true, Threads: 1}, io.Discard)
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
		DownloadOptions{Language: "en", Platform: "windows", RomMLayout: true, Threads: 1}, io.Discard)
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
		DownloadOptions{Language: "en", Platform: "windows", Flatten: true, Threads: 1}, io.Discard)
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
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
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
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
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
		DownloadOptions{Language: "en", Platform: "windows", Extras: true, Threads: 1}, io.Discard)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(tmp, "extragame", "windows", "setup.exe"))
	// enqueueExtras: SanitizePath("Game Soundtrack") = "game-soundtrack", then appends ".zip" from URL
	assert.FileExists(t, filepath.Join(tmp, "extragame", "extras", "game-soundtrack.zip"))
}

// files.json beside metadata.json records what a download brought: exact
// bytes, the MD5 of what streamed in, and when. A later run keeps the entries
// of files it skipped and replaces the ones it fetched again.
func TestDownloadGameFiles_WritesAFileManifest(t *testing.T) {
	body := []byte("the bytes gog served")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/game/setup_game_1.0.exe"
	game := Game{
		Title: "mygame",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &rawURL, Name: "setup.exe"}}},
		}},
	}

	require.NoError(t, DownloadGameFiles(context.Background(), "token", game, tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard))

	manifest, err := os.ReadFile(filepath.Join(tmp, "mygame", "files.json"))
	require.NoError(t, err)
	var entries []DownloadedFile
	require.NoError(t, json.Unmarshal(manifest, &entries))
	require.Len(t, entries, 1)

	sum := md5.Sum(body)
	require.Equal(t, filepath.Join("windows", "setup.exe"), entries[0].Path)
	require.Equal(t, int64(len(body)), entries[0].SizeBytes)
	require.Equal(t, hex.EncodeToString(sum[:]), entries[0].MD5, "the checksum is of what was written")
	require.WithinDuration(t, time.Now(), entries[0].DownloadedAt, time.Minute)
}

// A resumed download hashes the prefix it already holds, so the recorded
// checksum is of the whole file, not of the part that arrived this run.
func TestDownloadGameFiles_ManifestChecksumSurvivesResume(t *testing.T) {
	body := []byte("0123456789abcdef")
	var sawRange atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasPrefix(r.Header.Get("Range"), "bytes=") {
			sawRange.Store(true)
			var from int
			_, _ = fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &from)
			w.Header().Set("Content-Range",
				fmt.Sprintf("bytes %d-%d/%d", from, len(body)-1, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[from:])
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	tmp := t.TempDir()
	rawURL := server.URL + "/game/setup_game_1.0.exe"
	game := Game{
		Title: "mygame",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &rawURL, Name: "setup.exe"}}},
		}},
	}

	// Half the file is already on disk from an interrupted run.
	gameDir := filepath.Join(tmp, "mygame", "windows")
	require.NoError(t, os.MkdirAll(gameDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(gameDir, "setup.exe.part"), body[:8], 0o644))

	require.NoError(t, DownloadGameFiles(context.Background(), "token", game, tmp,
		DownloadOptions{Language: "en", Platform: "windows", Resume: true, Threads: 1}, io.Discard))

	manifest, err := os.ReadFile(filepath.Join(tmp, "mygame", "files.json"))
	require.NoError(t, err)
	var entries []DownloadedFile
	require.NoError(t, json.Unmarshal(manifest, &entries))
	require.Len(t, entries, 1)

	require.True(t, sawRange.Load(), "the run has to have resumed, not refetched")
	sum := md5.Sum(body)
	require.Equal(t, hex.EncodeToString(sum[:]), entries[0].MD5)
	require.Equal(t, int64(len(body)), entries[0].SizeBytes)
}

// A dropped connection or a server-side error is worth another try; a refusal
// such as 404 is not.
func TestDownloadGameFiles_RetriesTransientFailures(t *testing.T) {
	body := []byte("eventually served")
	var gets atomic.Int64
	var base string
	// The first GET of a file is the redirect probe; the download itself lands
	// on /dl, so only /dl requests count as tries.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dl/setup.exe" {
			w.Header().Set("Location", base+"/dl/setup.exe")
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.WriteHeader(http.StatusOK)
			return
		}
		if gets.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()
	base = server.URL

	tmp := t.TempDir()
	rawURL := server.URL + "/game/setup.exe"
	game := Game{
		Title: "mygame",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &rawURL, Name: "setup.exe"}}},
		}},
	}

	require.NoError(t, DownloadGameFiles(context.Background(), "token", game, tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard))

	data, err := os.ReadFile(filepath.Join(tmp, "mygame", "windows", "setup.exe"))
	require.NoError(t, err)
	require.Equal(t, body, data, "the third try brought the file")
	require.Equal(t, int64(3), gets.Load())
}

func TestDownloadGameFiles_DoesNotRetryARefusal(t *testing.T) {
	var gets atomic.Int64
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dl/setup.exe" {
			w.Header().Set("Location", base+"/dl/setup.exe")
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "8")
			w.WriteHeader(http.StatusOK)
			return
		}
		gets.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	base = server.URL

	tmp := t.TempDir()
	rawURL := server.URL + "/game/setup.exe"
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
	require.Equal(t, int64(1), gets.Load(), "asking again cannot turn a 404 into a file")
}
