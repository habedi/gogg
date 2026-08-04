package gui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// richGameData offers two languages, three platforms, a DLC and an extra.
const richGameData = `{"title":"Rich Game","downloads":[
	["English",{"windows":[{"manualUrl":"/w","name":"setup.exe","version":"2.1","size":"3 GB"}],
	            "linux":[{"manualUrl":"/l","name":"setup.sh","size":"3 GB"}]}],
	["Deutsch",{"mac":[{"manualUrl":"/m","name":"setup.dmg","size":"3 GB"}]}]],
	"extras":[{"name":"soundtrack","size":"500 MB","manualUrl":"/s"}],
	"dlcs":[{"title":"Expansion","downloads":[],"extras":[]}]}`

func detailValue(t *testing.T, details []gameDetail, label string) string {
	t.Helper()
	for _, detail := range details {
		if detail.Label == label {
			return detail.Value
		}
	}
	t.Fatalf("no detail labelled %q in %+v", label, details)
	return ""
}

func TestOfferedLanguagesAndPlatforms(t *testing.T) {
	parsed, err := client.ParseGameData(richGameData)
	require.NoError(t, err)

	require.Equal(t, []string{"Deutsch", "English"}, offeredLanguages(parsed))
	require.Equal(t, []string{"Windows", "Mac", "Linux"}, offeredPlatforms(parsed),
		"platforms are listed in a fixed order rather than alphabetically")
}

func TestOfferedPlatforms_OnlyListsWhatHasFiles(t *testing.T) {
	parsed, err := client.ParseGameData(
		`{"title":"X","downloads":[["English",{"windows":[{"manualUrl":"/w","name":"s.exe","size":"1 GB"}]}]],"extras":[],"dlcs":[]}`)
	require.NoError(t, err)

	require.Equal(t, []string{"Windows"}, offeredPlatforms(parsed))
}

// The pane answers the questions the list cannot: what this game is, what it
// ships with, and whether it is already on disk.
func TestGameDetails_DescribesTheGame(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	prefs := app.Preferences()
	prefs.SetString("downloadForm.language", "en")
	prefs.SetString("downloadForm.platform", "windows")
	prefs.SetBool("downloadForm.extras", false)
	prefs.SetBool("downloadForm.dlcs", false)

	updateStatusCache = make(map[int]updateStatus)
	t.Cleanup(func() { updateStatusCache = make(map[int]updateStatus) })

	game := db.Game{ID: 42, Title: "Rich Game", Data: richGameData, Version: "2.1"}
	details := gameDetails(game, &DownloadManager{Tasks: binding.NewUntypedList()})

	require.Equal(t, "42", detailValue(t, details, "Game ID"))
	require.Equal(t, "2.1", detailValue(t, details, "Version"))
	require.Equal(t, "Deutsch, English", detailValue(t, details, "Languages"))
	require.Equal(t, "Windows, Mac, Linux", detailValue(t, details, "Platforms"))
	require.Equal(t, "1", detailValue(t, details, "DLCs"))
	require.Equal(t, "1", detailValue(t, details, "Extras"))
	require.Equal(t, "3.0 GiB", detailValue(t, details, "Estimated size"))
	require.Equal(t, "No", detailValue(t, details, "Downloaded"))
}

func TestGameDetails_ShowsWhereADownloadedGameLives(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	updateStatusCache = make(map[int]updateStatus)
	t.Cleanup(func() { updateStatusCache = make(map[int]updateStatus) })

	root := t.TempDir()
	dir := filepath.Join(root, client.SanitizePath("Rich Game"))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(richGameData), 0o644))
	app.Preferences().SetString("lastUsedDownloadPath", root)

	game := db.Game{ID: 42, Title: "Rich Game", Data: richGameData}
	details := gameDetails(game, &DownloadManager{Tasks: binding.NewUntypedList()})

	require.Equal(t, dir, detailValue(t, details, "Downloaded to"))
}

func TestGameDetails_ReportsAWaitingUpdate(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	updateStatusCache = map[int]updateStatus{
		42: {Downloaded: true, HasUpdate: true, Diff: []string{"CHANGED: a", "CHANGED: b"}},
	}
	t.Cleanup(func() { updateStatusCache = make(map[int]updateStatus) })

	details := gameDetails(db.Game{ID: 42, Title: "Rich Game", Data: richGameData},
		&DownloadManager{Tasks: binding.NewUntypedList()})

	require.Equal(t, "2 changed files", detailValue(t, details, "Update"))
}

// Unparseable data must still describe what it can rather than showing nothing.
func TestGameDetails_SurvivesUnreadableGameData(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	details := gameDetails(db.Game{ID: 7, Title: "Broken", Data: "{not json"},
		&DownloadManager{Tasks: binding.NewUntypedList()})

	require.Equal(t, "7", detailValue(t, details, "Game ID"))
	require.Equal(t, "Unknown", detailValue(t, details, "Version"))
}

func TestRenderGameDetails_ShowsEveryRow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	rendered := renderGameDetails([]gameDetail{{"Game ID", "42"}, {"Version", "2.1"}})

	form, ok := rendered.(*widget.Form)
	require.True(t, ok)
	require.Len(t, form.Items, 2)
	require.Equal(t, "Game ID", form.Items[0].Text)
}

// Selecting a game has to fill the pane with that game's facts.
func TestLibraryTab_SelectingAGameFillsTheDetailsPane(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)

	before := len(widgetsOfType[*widget.Form](lt.content))

	// The list drives this binding when a row is clicked.
	require.NotNil(t, lt.selected)
	require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: richGameData, Version: "2.1"}))

	require.Greater(t, len(widgetsOfType[*widget.Form](lt.content)), before,
		"the details pane must gain a form of facts")
}

// A window cannot be smaller than its content, so a details pane taller than
// the default window pushes the download button off the screen.
func TestLibraryTab_FitsInADefaultWindow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 3)
	require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Rich Game", Data: richGameData, Version: "2.1"}))

	// The defaults in window.go.
	const defaultWidth, defaultHeight = 960, 640
	min := lt.content.MinSize()
	t.Logf("library minimum size: %.0fx%.0f", min.Width, min.Height)

	require.LessOrEqual(t, min.Height, float32(defaultHeight),
		"the library must fit in the default window height")
	require.LessOrEqual(t, min.Width, float32(defaultWidth),
		"the library must fit in the default window width")
}

func accordionWithItem(t *testing.T, root fyne.CanvasObject, title string) *widget.AccordionItem {
	t.Helper()
	for _, accordion := range widgetsOfType[*widget.Accordion](root) {
		for _, item := range accordion.Items {
			if item.Title == title {
				return item
			}
		}
	}
	t.Fatalf("no accordion section titled %q", title)
	return nil
}

// The facts are reference material; the pane should lead with the artwork and
// the thing you came to do.
func TestLibraryTab_GameDetailsStartCollapsed(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)

	require.False(t, accordionWithItem(t, lt.content, "Game Details").Open)
	require.True(t, accordionWithItem(t, lt.content, "Download Options").Open)
}

// Artwork leads the pane, above the facts and the download options.
func TestLibraryTab_ShowsArtworkAboveTheSections(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)
	require.NotNil(t, lt.artwork, "the details pane carries a place for artwork")

	order := paneOrder(t, lt)
	require.Equal(t, []string{"artwork", "Game Details", "Download Options"}, order)
}

// paneOrder names the details-pane sections from top to bottom.
func paneOrder(t *testing.T, lt *libraryTab) []string {
	t.Helper()
	var order []string
	walkWidgets(lt.content, func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *widget.Accordion:
			for _, item := range v.Items {
				order = append(order, item.Title)
			}
		case *canvas.Image:
			if v == lt.artwork {
				order = append(order, "artwork")
			}
		}
	})
	return order
}
