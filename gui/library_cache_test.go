package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// sizedGameData describes a game with a differently sized download per platform.
const sizedGameData = `{"title":"Sized","downloads":[["English",{` +
	`"windows":[{"manualUrl":"/w","name":"w.bin","size":"1 GB"}],` +
	`"linux":[{"manualUrl":"/l","name":"l.sh","size":"4 GB"}]}]],"extras":[],"dlcs":[]}`

// Estimated sizes depend on the language, platform, extras and DLC settings,
// so the cache must not serve a size computed under different settings.
func TestEstimateGameSize_FollowsPreferences(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	prefs := app.Preferences()

	game := db.Game{ID: 90001, Title: "Sized", Data: sizedGameData}

	prefs.SetString("downloadForm.language", "en")
	prefs.SetBool("downloadForm.extras", false)
	prefs.SetBool("downloadForm.dlcs", false)

	prefs.SetString("downloadForm.platform", "windows")
	windows := estimateGameSize(game)
	require.Equal(t, int64(1024*1024*1024), windows)

	prefs.SetString("downloadForm.platform", "linux")
	linux := estimateGameSize(game)
	require.Equal(t, int64(4*1024*1024*1024), linux,
		"the size must be recomputed when the platform changes")

	// Going back must still be served correctly (and from the cache).
	prefs.SetString("downloadForm.platform", "windows")
	require.Equal(t, windows, estimateGameSize(game))
}
