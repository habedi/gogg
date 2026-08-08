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
