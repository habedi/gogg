package gui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// A game is found under the path the user typed into the download form, not
// only under the path the last actual download wrote. The two used to live
// in different preference keys, so a typed-but-not-yet-downloaded path was
// invisible to this lookup.
func TestGetGameDownloadDirectory_FindsTheFormTypedPath(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := t.TempDir()
	dir := filepath.Join(root, client.SanitizePath("Typed Game"))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{}"), 0o644))

	// The form's key is set; the legacy download key is not.
	app.Preferences().SetString("downloadForm.path", root)
	app.Preferences().RemoveValue("lastUsedDownloadPath")

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	found, ok := getGameDownloadDirectory(dm, db.Game{ID: 1, Title: "Typed Game"})
	require.True(t, ok, "the game under the typed path has to be found")
	require.Equal(t, dir, found)
}

func TestGetGameDownloadDirectory_LutrisAndRomMLayouts(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := t.TempDir()
	app.Preferences().SetString("downloadForm.path", root)

	// Test Lutris layout
	lutrisGame := db.Game{ID: 10, Title: "Lutris Game"}
	lutrisDir := filepath.Join(root, client.LutrisSlug(lutrisGame.Title), "gog")
	require.NoError(t, os.MkdirAll(lutrisDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lutrisDir, "metadata.json"), []byte("{}"), 0o644))

	found, ok := getGameDownloadDirectory(nil, lutrisGame)
	require.True(t, ok, "lutris layout should be discovered")
	require.Equal(t, lutrisDir, found)

	// Test RomM layout
	rommGame := db.Game{ID: 20, Title: "RomM Game"}
	rommDir := filepath.Join(root, "win", client.SanitizePath(rommGame.Title))
	require.NoError(t, os.MkdirAll(rommDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rommDir, "metadata.json"), []byte("{}"), 0o644))

	found, ok = getGameDownloadDirectory(nil, rommGame)
	require.True(t, ok, "romm layout should be discovered")
	require.Equal(t, rommDir, found)
}

func TestHistory_NilDownloadManagerSafety(t *testing.T) {
	require.False(t, isGameDownloaded(nil, 1))
	dir, ok := getLastCompletedDownloadDir(nil, 1)
	require.False(t, ok)
	require.Empty(t, dir)
}
