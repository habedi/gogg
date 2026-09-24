package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func singleFileGame(t *testing.T, url string) Game {
	t.Helper()
	game, err := ParseGameData(`{"title":"Some Game","downloads":[["English",{"windows":[` +
		`{"manualUrl":"` + url + `","name":"setup.bin","size":"15 B"}]}]],"extras":[],"dlcs":[]}`)
	require.NoError(t, err)
	return game
}

func fileServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "15")
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte("installer-bytes"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// In the RomM layout the files land in <root>/<platform>/<game>, and the
// metadata the GUI reads back for update detection has to be there with them.
func TestDownloadGameFiles_RommLayoutStoresMetadataWithTheFiles(t *testing.T) {
	srv := fileServer(t)
	root := t.TempDir()

	require.NoError(t, DownloadGameFiles(context.Background(), "tok", singleFileGame(t, srv.URL+"/setup.bin"), root,
		DownloadOptions{Language: "English", Platform: "windows", RomMLayout: true, Threads: // rommLayout
		1}, io.Discard))

	gameDir := filepath.Join(root, "win", "some-game")
	require.FileExists(t, filepath.Join(gameDir, "setup.bin"))
	require.FileExists(t, filepath.Join(gameDir, "metadata.json"))
	require.NoDirExists(t, filepath.Join(root, "some-game"),
		"no game directory may be created outside the RomM layout")
}

// The default layout is unchanged: metadata sits in the game directory.
func TestDownloadGameFiles_DefaultLayoutStoresMetadataInTheGameDirectory(t *testing.T) {
	srv := fileServer(t)
	root := t.TempDir()

	require.NoError(t, DownloadGameFiles(context.Background(), "tok", singleFileGame(t, srv.URL+"/setup.bin"), root,
		DownloadOptions{Language: "English", Platform: "windows", Threads: // rommLayout
		1}, io.Discard))

	require.FileExists(t, filepath.Join(root, "some-game", "metadata.json"))
}
