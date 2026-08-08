package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func TestLanguageChoices_OffersWhatTheGameShipsIn(t *testing.T) {
	require.Equal(t, []string{"Deutsch", "English"},
		languageChoices([]string{"English", "Deutsch"}))
}

// GOG ships games in languages gogg has no code for. Offering nothing at all
// would be worse than offering everything.
func TestLanguageChoices_FallsBackWhenNoneAreKnown(t *testing.T) {
	choices := languageChoices([]string{"Čeština", "Magyar"})
	require.Contains(t, choices, "English")
	require.Greater(t, len(choices), 5)
}

func TestLanguageChoices_OffersEverythingWhenNothingIsKnownAboutTheGame(t *testing.T) {
	require.Equal(t, languageChoices(nil), languageChoices([]string{}))
	require.Contains(t, languageChoices(nil), "Português do Brasil")
}

func TestPlatformChoices(t *testing.T) {
	require.Equal(t, []string{"windows", "linux", "all"},
		platformChoices([]string{"Windows", "Linux"}),
		"the values stay lowercase, and all is always on offer")

	require.Equal(t, []string{"windows", "mac", "linux", "all"}, platformChoices(nil))
}

// Narrowing the choices for a game is not the user choosing, so it must not
// overwrite what they picked as their default.
func TestBindSelect_DoesNotFireForAProgrammaticChange(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	var chosen []string
	sel := widget.NewSelect(nil, nil)

	bindSelect(sel, []string{"English", "Deutsch"}, "English", func(s string) { chosen = append(chosen, s) })
	require.Equal(t, "English", sel.Selected)
	require.Empty(t, chosen)

	// A game that only ships in German.
	bindSelect(sel, []string{"Deutsch"}, "English", func(s string) { chosen = append(chosen, s) })
	require.Equal(t, "Deutsch", sel.Selected, "an unavailable choice falls back to the first on offer")
	require.Empty(t, chosen, "narrowing the list is not a user choice")

	// The user picking something is. (SetSelected fires OnChanged, which is
	// exactly why bindSelect detaches it first.)
	bindSelect(sel, []string{"English", "Deutsch"}, "Deutsch", func(s string) { chosen = append(chosen, s) })
	require.Empty(t, chosen)
	sel.SetSelected("English")
	require.Equal(t, []string{"English"}, chosen)
}

func TestBindSelect_KeepsTheWantedValueWhenItIsOffered(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	sel := widget.NewSelect(nil, nil)
	bindSelect(sel, []string{"English", "Deutsch", "Polski"}, "Polski", nil)
	require.Equal(t, "Polski", sel.Selected)
}

// Selecting a game restricts the form to what that game actually offers.
func TestLibraryTab_SelectingAGameNarrowsTheOptions(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Preferences().SetString("downloadForm.language", "en")

	lt, _ := newLibraryFixture(t, 2)

	// A game published only for Windows, only in German.
	germanOnly := `{"title":"Nur Deutsch","downloads":[["Deutsch",{"windows":[` +
		`{"manualUrl":"/w","name":"setup.exe","size":"1 GB"}]}]],"extras":[],"dlcs":[]}`
	require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Nur Deutsch", Data: germanOnly}))

	language := checkGroupWithOption(t, lt.content, "Deutsch")
	require.Equal(t, []string{"Deutsch"}, language.Options)
	require.Equal(t, []string{"Deutsch"}, language.Selected,
		"a wanted language the game lacks falls back to what it has")

	platform := checkGroupWithOption(t, lt.content, "windows")
	require.Equal(t, []string{"windows"}, platform.Options,
		"no all box: in check boxes, all is every box ticked")

	require.Equal(t, "en", app.Preferences().String("downloadForm.language"),
		"narrowing the list must not rewrite the user's default language")
}

// Narrowing the boxes for a game is not the user choosing.
func TestBindCheckGroup_DoesNotFireForAProgrammaticChange(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	var fired [][]string
	group := widget.NewCheckGroup(nil, nil)

	bindCheckGroup(group, []string{"English", "Deutsch", "Polski"},
		[]string{"Deutsch", "Polski"}, func(chosen []string) { fired = append(fired, chosen) })
	require.Equal(t, []string{"Deutsch", "Polski"}, group.Selected, "offered choices are kept")
	require.Empty(t, fired)

	bindCheckGroup(group, []string{"English"}, []string{"Deutsch"}, nil)
	require.Equal(t, []string{"English"}, group.Selected,
		"nothing wanted on offer ticks the first box: a download needs a language")
	require.Empty(t, fired)
}
