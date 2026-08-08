package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func TestLanguageChoices_OffersWhatTheGameShipsIn(t *testing.T) {
	// English leads when offered; the rest follow in alphabetical order.
	require.Equal(t, []string{"English", "Deutsch"},
		languageChoices([]string{"English", "Deutsch"}))
	require.Equal(t, []string{"English", "Deutsch", "Français"},
		languageChoices([]string{"Français", "Deutsch", "English"}))
	// A game without English keeps plain alphabetical order.
	require.Equal(t, []string{"Deutsch", "Français"},
		languageChoices([]string{"Français", "Deutsch"}))
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

	grids := widgetsOfType[*checkGrid](lt.content)
	require.Len(t, grids, 1, "the languages are a check grid")
	language := grids[0]
	require.Equal(t, []string{"Deutsch"}, language.options)
	require.Equal(t, []string{"Deutsch"}, language.selected(),
		"a wanted language the game lacks falls back to what it has")

	platform := checkGroupWithOption(t, lt.content, "Windows")
	require.Equal(t, []string{"Windows"}, platform.Options,
		"the boxes show platform names, and there is no all box: all is every box ticked")

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

// The two special layouts contradict each other: ticking one clears the other.
func TestDownloadForm_LayoutsExcludeEachOther(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 1)

		romm := checkWithLabel(lt.content, "RomM folder layout (platform/game)")
		lutris := checkWithLabel(lt.content, "Lutris cache layout (game/gog)")
		require.NotNil(t, romm)
		require.NotNil(t, lutris)

		lutris.SetChecked(true)
		romm.SetChecked(true)
		require.False(t, lutris.Checked, "ticking RomM clears Lutris")

		lutris.SetChecked(true)
		require.False(t, romm.Checked, "and the other way round")
		require.True(t, app.Preferences().Bool("downloadForm.lutris"))
		require.False(t, app.Preferences().Bool("downloadForm.romm"))
	})
}

// The connections select splits large files across range requests; one is
// the default, so nothing changes for anyone who does not ask.
func TestDownloadForm_OffersConnectionsPerFile(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 1)

	var connections *widget.Select
	for _, sel := range widgetsOfType[*widget.Select](lt.content) {
		if len(sel.Options) > 0 && sel.Options[len(sel.Options)-1] == "8" {
			connections = sel
		}
	}
	require.NotNil(t, connections, "the form offers a connections select")
	require.Equal(t, []string{"1", "2", "4", "8"}, connections.Options)
	require.Equal(t, "1", connections.Selected, "one connection is the default")

	connections.SetSelected("4")
	require.Equal(t, "4", app.Preferences().String("downloadForm.connections"))
}

// The platform boxes read as Windows, macOS, and Linux, while the values
// underneath stay the keys downloads run on.
func TestPlatformGroupLabels_ShowProperNames(t *testing.T) {
	require.Equal(t, []string{"Windows", "Linux"}, platformGroupLabels([]string{"Windows", "Linux"}))
	require.Equal(t, []string{"Windows", "macOS", "Linux"}, platformGroupLabels(nil))

	// The labels map back to the keys the download options take.
	require.Equal(t, []string{"windows", "mac", "linux"},
		platformKeys([]string{"Windows", "macOS", "Linux"}))
	// And the stored keys map to labels for pre-ticking the boxes.
	require.Equal(t, []string{"Windows", "macOS", "Linux"},
		platformLabels([]string{"windows", "mac", "linux"}))
}

// The languages are laid out in a three-column grid rather than one tall
// column, so a game that ships in many of them stays readable.
func TestLibraryTab_LanguagesAreAThreeColumnGrid(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 1)
	manyLangs := `{"title":"Many","downloads":[
		["English",{"windows":[{"manualUrl":"/e","name":"s.exe","size":"1 GB"}]}],
		["Deutsch",{"windows":[{"manualUrl":"/d","name":"s.exe","size":"1 GB"}]}],
		["Français",{"windows":[{"manualUrl":"/f","name":"s.exe","size":"1 GB"}]}],
		["Español",{"windows":[{"manualUrl":"/s","name":"s.exe","size":"1 GB"}]}]],
		"extras":[],"dlcs":[]}`
	require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Many", Data: manyLangs}))

	grids := widgetsOfType[*checkGrid](lt.content)
	require.Len(t, grids, 1)
	require.Equal(t, 3, grids[0].columns, "the languages sit in three columns")
	require.ElementsMatch(t, []string{"Deutsch", "English", "Español", "Français"}, grids[0].options)
}
