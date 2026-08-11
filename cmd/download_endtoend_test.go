package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// A download run the whole way through the command, with nothing standing in
// for anything: the catalogue and the token are in the real database, the
// files come from a real HTTP server, and the bytes land on a real disk.

// downloadFixture serves one installer and puts the game that names it in the
// catalogue, next to a token that has not expired. It returns the game's ID
// and the directory the download is to land in.
func downloadFixture(t *testing.T, body []byte) (gameID int, downloadDir string) {
	t.Helper()
	// A database of its own rather than the one TestMain opened:
	// TestInitializeAndCloseDatabase closes that one partway through the
	// package, and these tests sort after it.
	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	// Nothing here may reach the real GOG API, whatever a lookup decides to do.
	t.Setenv("GOGG_API_BASE", srv.URL)
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	ctx := context.Background()
	tokens := db.NewTokenRepository(db.GetDB())
	require.NoError(t, tokens.Upsert(ctx, &db.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour).Format(time.RFC3339),
	}))

	data := fmt.Sprintf(`{"title":"God of War","downloads":[["English",{"windows":`+
		`[{"manualUrl":"%s/downloads/god_of_war/en1installer0","name":"setup.exe","size":"1 MB"}]}]],`+
		`"extras":[],"dlcs":[]}`, srv.URL)
	require.NoError(t, db.NewGameRepository(db.GetDB()).Put(ctx, db.Game{ID: 42, Title: "God of War", Data: data}))

	return 42, filepath.Join(t.TempDir(), "games")
}

// authFromDB is the real service reading the real token table. A token that
// has not expired is handed back as it is, so no refresher is needed.
func authFromDB() *auth.Service {
	return auth.NewServiceWithRepo(db.NewTokenRepository(db.GetDB()), nil)
}

func TestExecuteDownload_BringsTheFileAndTheManifest(t *testing.T) {
	body := []byte("the installer's bytes")
	gameID, dir := downloadFixture(t, body)

	captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "windows",
			false, false, false, false, false, false, false, false, true, 1, 1)
	})

	landed, err := os.ReadFile(filepath.Join(dir, "god-of-war", "windows", "setup.exe"))
	require.NoError(t, err)
	require.Equal(t, body, landed)

	require.FileExists(t, filepath.Join(dir, "god-of-war", "metadata.json"))
	require.FileExists(t, filepath.Join(dir, "god-of-war", "files.json"))
}

func TestExecuteDownload_SaysWhenVerificationIsOff(t *testing.T) {
	gameID, dir := downloadFixture(t, []byte("unverified bytes"))

	out := captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "windows",
			false, false, false, false, false, false, false, false, true, 1, 1)
	})
	require.Contains(t, out, "Checksum verification is off")

	out = captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "windows",
			false, false, true, false, false, false, false, false, false, 1, 1)
	})
	require.NotContains(t, out, "Checksum verification is off",
		"a download that verifies says nothing about it")
}

func TestExecuteDownload_FlattenPutsTheFileInTheGameFolder(t *testing.T) {
	gameID, dir := downloadFixture(t, []byte("flat bytes"))

	captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "windows",
			false, false, false, true, false, false, false, false, true, 1, 1)
	})

	require.FileExists(t, filepath.Join(dir, "god-of-war", "setup.exe"))
}

func TestExecuteDownload_LutrisLayoutUsesTheCacheFolder(t *testing.T) {
	gameID, dir := downloadFixture(t, []byte("lutris bytes"))

	captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "windows",
			false, false, false, false, false, false, false, true, true, 1, 1)
	})

	require.FileExists(t, filepath.Join(dir, "god-of-war", "gog", "setup.exe"))
}

func TestExecuteDownload_RommLayoutUsesThePlatformFolder(t *testing.T) {
	gameID, dir := downloadFixture(t, []byte("romm bytes"))

	captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "windows",
			false, false, false, false, false, false, true, false, true, 1, 1)
	})

	require.FileExists(t, filepath.Join(dir, "windows", "god-of-war", "setup.exe"))
}

func TestExecuteDownload_ReportsAGameThatIsNotInTheCatalogue(t *testing.T) {
	_, dir := downloadFixture(t, []byte("never asked for"))

	out := captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), 999, dir, "en", "windows",
			false, false, false, false, false, false, false, false, true, 1, 1)
	})
	require.Contains(t, out, "not found in local catalogue")
}

func TestExecuteDownload_ReportsAMissingToken(t *testing.T) {
	gameID, dir := downloadFixture(t, []byte("no token, no download"))
	require.NoError(t, db.NewTokenRepository(db.GetDB()).Delete(context.Background()))

	out := captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "windows",
			false, false, false, false, false, false, false, false, true, 1, 1)
	})
	require.Contains(t, out, "Did you login?")
}

func TestExecuteDownload_RejectsBadCounts(t *testing.T) {
	gameID, dir := downloadFixture(t, []byte("never fetched"))

	out := captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "windows",
			false, false, false, false, false, false, false, false, true, 999, 1)
	})
	require.Contains(t, out, "Invalid thread count")

	out = captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "windows",
			false, false, false, false, false, false, false, false, true, 1, 99)
	})
	require.Contains(t, out, "Invalid connection count")

	out = captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "amiga",
			false, false, false, false, false, false, false, false, true, 1, 1)
	})
	require.Contains(t, out, "Invalid platform")
}

func TestExecuteDownload_RefusesBothLayoutsAtOnce(t *testing.T) {
	gameID, dir := downloadFixture(t, []byte("one layout at a time"))

	out := captureStdout2(func() {
		executeDownload(context.Background(), authFromDB(), gameID, dir, "en", "windows",
			false, false, false, false, false, false, true, true, true, 1, 1)
	})
	require.Contains(t, out, "Failed to download game files")
}
