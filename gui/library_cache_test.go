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

	state := newLibraryState()
	game := db.Game{ID: 90001, Title: "Sized", Data: sizedGameData}

	prefs.SetString("downloadForm.language", "en")
	prefs.SetBool("downloadForm.extras", false)
	prefs.SetBool("downloadForm.dlcs", false)

	prefs.SetString("downloadForm.platform", "windows")
	windows := state.estimateSize(game)
	require.Equal(t, int64(1024*1024*1024), windows)

	prefs.SetString("downloadForm.platform", "linux")
	linux := state.estimateSize(game)
	require.Equal(t, int64(4*1024*1024*1024), linux,
		"the size must be recomputed when the platform changes")

	// Going back must still be served correctly (and from the cache).
	prefs.SetString("downloadForm.platform", "windows")
	require.Equal(t, windows, state.estimateSize(game))
}

// The size a game is filtered by does not depend on the download-form
// settings: picking Linux must not shrink a Windows game out of the "large
// games" filter. This was the bug where choosing Linux left only a handful
// of games looking large.
func TestCatalogueSize_IgnoresTheFormSettings(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	// A game whose big installer is Windows, with only a tiny Linux build.
	game := db.Game{ID: 1, Title: "Big On Windows", Data: `{"title":"Big On Windows","downloads":[
		["English",{
			"windows":[{"manualUrl":"/w","name":"w.exe","size":"40 GB"}],
			"linux":[{"manualUrl":"/l","name":"l.sh","size":"200 MB"}]}]],
		"extras":[],"dlcs":[]}`}

	state := newLibraryState()

	app.Preferences().SetString("downloadForm.platform", "linux")
	linuxView := state.catalogueSize(game)

	state.forgetParsed()
	app.Preferences().SetString("downloadForm.platform", "windows")
	windowsView := state.catalogueSize(game)

	require.Equal(t, windowsView, linuxView, "the catalogue size is the same whatever platform is picked")
	require.Greater(t, linuxView, int64(30)<<30, "and it reflects the game's largest platform, not the smallest")
}
