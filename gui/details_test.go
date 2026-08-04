package gui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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

	var forms []*widget.Form
	collectForms(lt.content, &forms)
	before := len(forms)

	// The list drives this binding when a row is clicked.
	require.NotNil(t, lt.selected)
	require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: richGameData, Version: "2.1"}))

	forms = nil
	collectForms(lt.content, &forms)
	require.Greater(t, len(forms), before, "the details pane must gain a form of facts")
}

func collectForms(o fyne.CanvasObject, out *[]*widget.Form) {
	switch v := o.(type) {
	case *widget.Form:
		*out = append(*out, v)
	case *fyne.Container:
		for _, c := range v.Objects {
			collectForms(c, out)
		}
	case *widget.Card:
		if v.Content != nil {
			collectForms(v.Content, out)
		}
	case *container.Split:
		collectForms(v.Leading, out)
		collectForms(v.Trailing, out)
	case *widget.Accordion:
		for _, item := range v.Items {
			collectForms(item.Detail, out)
		}
	}
}
