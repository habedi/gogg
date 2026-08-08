package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The slugs must match Lutris's slugify exactly: a folder that differs by one
// character is a cache miss and a re-download for Lutris. The expected values
// here were produced by running Lutris's own Python implementation.
func TestLutrisSlug_MatchesLutris(t *testing.T) {
	for title, want := range map[string]string{
		"The Witcher® 3: Wild Hunt":           "the-witcher-3-wild-hunt",
		"S.T.A.L.K.E.R.: Shadow of Chornobyl": "stalker-shadow-of-chornobyl",
		"Heroes of Might and Magic® 3":        "heroes-of-might-and-magic-3",
		"Pokémon Über":                        "pokemon-uber",
		"Baldur's Gate":                       "baldurs-gate",
		"already-a-slug":                      "already-a-slug",
		"  spaced   out  ":                    "spaced-out",
	} {
		require.Equal(t, want, LutrisSlug(title), "title %q", title)
	}
}

// A name that slugifies to nothing becomes the same version-5 UUID Lutris
// falls back to.
func TestLutrisSlug_NonLatinNameBecomesTheLutrisUUID(t *testing.T) {
	require.Equal(t, "7b683cac-c8cc-5de2-84a2-2abbcda239ff", LutrisSlug("ポケモン"),
		"the UUID has to match what Lutris's uuid5 produces")
}

// The periods that survive gogg's own folder names do not survive a Lutris
// slug; the two layouts genuinely differ.
func TestLutrisSlug_IsNotSanitizePath(t *testing.T) {
	require.Equal(t, "s.t.a.l.k.e.r.-shadow-of-chornobyl", SanitizePath("S.T.A.L.K.E.R.: Shadow of Chornobyl"))
	require.Equal(t, "stalker-shadow-of-chornobyl", LutrisSlug("S.T.A.L.K.E.R.: Shadow of Chornobyl"))
}

// Under the Lutris layout every file lands flat in <slug>/gog, which is
// exactly where Lutris looks before downloading anything itself.
func TestDownloadGameFiles_LutrisLayout(t *testing.T) {
	body := []byte("cached for lutris")
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
	rawURL := server.URL + "/game/setup.exe"
	game := Game{
		Title: "The Witcher® 3: Wild Hunt",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &rawURL, Name: "setup.exe"}}},
		}},
	}

	require.NoError(t, DownloadGameFiles(context.Background(), "token", game, tmp,
		DownloadOptions{Language: "en", Platform: "windows", LutrisLayout: true, Threads: 1}, io.Discard))

	gogDir := filepath.Join(tmp, "the-witcher-3-wild-hunt", "gog")
	data, err := os.ReadFile(filepath.Join(gogDir, "setup.exe"))
	require.NoError(t, err)
	require.Equal(t, body, data, "the file lands flat in <slug>/gog, no platform folder")

	require.FileExists(t, filepath.Join(gogDir, "metadata.json"), "the metadata sits with the files")
	require.FileExists(t, filepath.Join(gogDir, "files.json"))
}

// The two special layouts contradict each other and are refused together.
func TestDownloadGameFiles_RefusesRomMAndLutrisTogether(t *testing.T) {
	err := DownloadGameFiles(context.Background(), "token", Game{Title: "g"}, t.TempDir(),
		DownloadOptions{Language: "en", Platform: "windows", RomMLayout: true, LutrisLayout: true, Threads: 1},
		io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot both")
}

// Pruning under the Lutris layout works on the <slug>/gog folder.
func TestPruneOldInstallerVersions_LutrisLayout(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"stalker-shadow-of-chornobyl/gog/setup_game_1.0.0.exe",
		"stalker-shadow-of-chornobyl/gog/setup_game_2.0.0.exe",
	)

	removed, err := PruneOldInstallerVersions(root, "S.T.A.L.K.E.R.: Shadow of Chornobyl",
		DownloadOptions{LutrisLayout: true})
	require.NoError(t, err)
	require.Len(t, removed, 1)

	assertExists(t, root, "stalker-shadow-of-chornobyl/gog/setup_game_2.0.0.exe")
	assertGone(t, root, "stalker-shadow-of-chornobyl/gog/setup_game_1.0.0.exe")
}
