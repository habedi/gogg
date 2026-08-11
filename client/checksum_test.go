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

	"github.com/stretchr/testify/require"
)

func TestDownlinkKind(t *testing.T) {
	for _, tc := range []struct {
		manualURL string
		kind      string
		fileID    string
	}{
		{"https://example.com/downloads/god_of_war/en1installer0", "installer", "en1installer0"},
		{"/downloads/game/en1patch2", "patch", "en1patch2"},
		{"/downloads/game/12345", "", ""},
		{"", "", ""},
	} {
		kind, fileID := downlinkKind(tc.manualURL)
		require.Equal(t, tc.kind, kind, tc.manualURL)
		if tc.kind != "" {
			require.Equal(t, tc.fileID, fileID, tc.manualURL)
		}
	}
}

// checksumFixture is a server that plays GOG's three roles at once: the file
// host, the downlink endpoint, and the checksum manifest. The manifest lies
// or tells the truth as the test decides.
func checksumFixture(t *testing.T, body []byte, manifestMD5 string, downlinkStatus int) (*httptest.Server, string) {
	t.Helper()
	return checksumFixtureNamed(t, body, "setup.exe", manifestMD5, downlinkStatus)
}

// checksumFixtureNamed is checksumFixture with a say in which file the
// manifest claims to be about, so a manifest for another file can be tested.
func checksumFixtureNamed(t *testing.T, body []byte, manifestName, manifestMD5 string, downlinkStatus int) (*httptest.Server, string) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/products/"):
			if downlinkStatus != http.StatusOK {
				w.WriteHeader(downlinkStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"downlink": srv.URL + "/file/setup.exe",
				"checksum": srv.URL + "/file/setup.exe.xml",
			})
		case strings.HasSuffix(r.URL.Path, ".xml"):
			_, _ = fmt.Fprintf(w, `<file name="%s" md5="%s" chunks="1" total_size="%d"/>`, manifestName, manifestMD5, len(body))
		default:
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusOK)
				return
			}
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GOGG_API_BASE", srv.URL)
	return srv, srv.URL + "/downloads/some_game/en1installer0"
}

func checksumGame(manualURL string) Game {
	return Game{
		ID:    42,
		Title: "Some Game",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &manualURL, Name: "setup.exe"}}},
		}},
	}
}

func TestDownloadGameFiles_RecordsAVerifiedChecksum(t *testing.T) {
	body := []byte("the real bytes")
	sum := md5.Sum(body)
	_, manualURL := checksumFixture(t, body, hex.EncodeToString(sum[:]), http.StatusOK)

	tmp := t.TempDir()
	require.NoError(t, DownloadGameFiles(context.Background(), "token", checksumGame(manualURL), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard))

	manifest, err := os.ReadFile(filepath.Join(tmp, "some-game", "files.json"))
	require.NoError(t, err)
	var files []DownloadedFile
	require.NoError(t, json.Unmarshal(manifest, &files))
	require.Len(t, files, 1)
	require.True(t, files[0].MD5Verified, "a matching checksum is recorded as verified")
}

func TestDownloadGameFiles_DeletesAFileGOGDisowns(t *testing.T) {
	body := []byte("bytes that do not match")
	_, manualURL := checksumFixture(t, body, "00000000000000000000000000000000", http.StatusOK)

	tmp := t.TempDir()
	err := DownloadGameFiles(context.Background(), "token", checksumGame(manualURL), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum mismatch")

	_, statErr := os.Stat(filepath.Join(tmp, "some-game", "windows", "setup.exe"))
	require.True(t, os.IsNotExist(statErr), "the mismatched file must not be left on disk")
}

func TestDownloadGameFiles_NoChecksumIsNoVerdict(t *testing.T) {
	body := []byte("fine, just unverifiable")
	_, manualURL := checksumFixture(t, body, "", http.StatusNotFound)

	tmp := t.TempDir()
	require.NoError(t, DownloadGameFiles(context.Background(), "token", checksumGame(manualURL), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard))

	manifest, err := os.ReadFile(filepath.Join(tmp, "some-game", "files.json"))
	require.NoError(t, err)
	var files []DownloadedFile
	require.NoError(t, json.Unmarshal(manifest, &files))
	require.Len(t, files, 1)
	require.False(t, files[0].MD5Verified)
	require.NotEmpty(t, files[0].MD5, "the streamed checksum is still recorded")
}

func TestDownloadGameFiles_SkipsLookupWithoutAProductID(t *testing.T) {
	var downlinkCalls atomic.Int32
	body := []byte("no id, no lookup")
	srv, manualURL := checksumFixture(t, body, "irrelevant", http.StatusOK)
	inner := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/products/") {
			downlinkCalls.Add(1)
		}
		inner.ServeHTTP(w, r)
	})

	game := checksumGame(manualURL)
	game.ID = 0
	require.NoError(t, DownloadGameFiles(context.Background(), "token", game, t.TempDir(),
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard))
	require.Zero(t, downlinkCalls.Load(), "a game whose product ID is unknown asks GOG nothing")
}

func TestDownloadGameFiles_SkipVerifyLeavesTheVerdictUnasked(t *testing.T) {
	var downlinkCalls atomic.Int32
	body := []byte("bytes GOG would disown")
	srv, manualURL := checksumFixture(t, body, "00000000000000000000000000000000", http.StatusOK)
	inner := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/products/") {
			downlinkCalls.Add(1)
		}
		inner.ServeHTTP(w, r)
	})

	tmp := t.TempDir()
	require.NoError(t, DownloadGameFiles(context.Background(), "token", checksumGame(manualURL), tmp,
		DownloadOptions{Language: "en", Platform: "windows", SkipVerify: true, Threads: 1}, io.Discard))
	require.Zero(t, downlinkCalls.Load(), "a download told not to verify asks GOG nothing")

	require.FileExists(t, filepath.Join(tmp, "some-game", "windows", "setup.exe"),
		"a file that would have failed verification is kept")

	manifest, err := os.ReadFile(filepath.Join(tmp, "some-game", "files.json"))
	require.NoError(t, err)
	var files []DownloadedFile
	require.NoError(t, json.Unmarshal(manifest, &files))
	require.Len(t, files, 1)
	require.False(t, files[0].MD5Verified)
	require.NotEmpty(t, files[0].MD5, "the streamed checksum is still recorded")
}

func TestManifestDescribes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		manifestName string
		manifestSize int64
		fileName     string
		size         int64
		want         bool
	}{
		{"same name", "setup.exe", 10, "setup.exe", 10, true},
		{"same name, different case", "Setup.exe", 10, "setup.exe", 10, true},
		{"another file's manifest", "setup_base_game.exe", 10, "setup_the_dlc.exe", 20, false},
		{"same name, different size", "setup.exe", 99, "setup.exe", 10, true},
		{"no name, same size", "", 10, "setup.exe", 10, true},
		{"no name, different size", "", 99, "setup.exe", 10, false},
		{"no name, no size", "", 0, "setup.exe", 10, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, manifestDescribes(tc.manifestName, tc.manifestSize, tc.fileName, tc.size))
		})
	}
}

func TestDownloadGameFiles_AManifestForAnotherFileIsNoVerdict(t *testing.T) {
	body := []byte("the real bytes of this file")
	_, manualURL := checksumFixtureNamed(t, body, "setup_another_game.exe",
		"00000000000000000000000000000000", http.StatusOK)

	tmp := t.TempDir()
	require.NoError(t, DownloadGameFiles(context.Background(), "token", checksumGame(manualURL), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard))

	manifest, err := os.ReadFile(filepath.Join(tmp, "some-game", "files.json"))
	require.NoError(t, err)
	var files []DownloadedFile
	require.NoError(t, json.Unmarshal(manifest, &files))
	require.Len(t, files, 1)
	require.False(t, files[0].MD5Verified, "a manifest about another file settles nothing")
	require.NotEmpty(t, files[0].MD5, "the streamed checksum is still recorded")
}

// dlcChecksumGame is a game whose only files belong to a DLC, the way
// Cyberpunk 2077's expansion is downloaded next to an installed base game.
func dlcChecksumGame(manualURL string) Game {
	return Game{
		ID:    42,
		Title: "Some Game",
		DLCs: []DLC{{
			Title: "The Expansion",
			ParsedDownloads: []Downloadable{{
				Language:  "en",
				Platforms: Platform{Windows: []PlatformFile{{ManualURL: &manualURL, Name: "setup_the_expansion.exe"}}},
			}},
		}},
	}
}

func TestDownloadGameFiles_DoesNotVerifyADLCAgainstTheBaseGame(t *testing.T) {
	var downlinkCalls atomic.Int32
	body := []byte("the expansion's bytes")
	// The base game's product would answer about its own file of the same ID,
	// whose checksum this download must not be held to.
	srv, manualURL := checksumFixtureNamed(t, body, "setup_the_base_game.exe",
		"00000000000000000000000000000000", http.StatusOK)
	inner := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/products/") {
			downlinkCalls.Add(1)
		}
		inner.ServeHTTP(w, r)
	})

	tmp := t.TempDir()
	require.NoError(t, DownloadGameFiles(context.Background(), "token", dlcChecksumGame(manualURL), tmp,
		DownloadOptions{Language: "en", Platform: "windows", DLCs: true, Threads: 1}, io.Discard))
	require.Zero(t, downlinkCalls.Load(), "a DLC's files have no product to be verified against")

	manifest, err := os.ReadFile(filepath.Join(tmp, "some-game", "files.json"))
	require.NoError(t, err)
	var files []DownloadedFile
	require.NoError(t, json.Unmarshal(manifest, &files))
	require.Len(t, files, 1)
	require.False(t, files[0].MD5Verified)
	require.NotEmpty(t, files[0].MD5, "the streamed checksum is still recorded")
}
