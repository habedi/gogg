package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"fyne.io/fyne/v2"
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
	details := gameDetails(game, &DownloadManager{Tasks: binding.NewUntypedList()}, nil)

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
	details := gameDetails(game, &DownloadManager{Tasks: binding.NewUntypedList()}, nil)

	require.Equal(t, dir, detailValue(t, details, "Downloaded to"))
}

func TestGameDetails_ReportsAWaitingUpdate(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	updateStatusCache = map[int]updateStatus{
		42: {Downloaded: true, HasUpdate: true, Diff: []string{"CHANGED: a", "CHANGED: b"}},
	}
	t.Cleanup(func() { updateStatusCache = make(map[int]updateStatus) })

	details := gameDetails(db.Game{ID: 42, Title: "Rich Game", Data: richGameData}, &DownloadManager{Tasks: binding.NewUntypedList()}, nil)

	require.Equal(t, "2 changed files", detailValue(t, details, "Update"))
}

// Unparseable data must still describe what it can rather than showing nothing.
func TestGameDetails_SurvivesUnreadableGameData(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	details := gameDetails(db.Game{ID: 7, Title: "Broken", Data: "{not json"}, &DownloadManager{Tasks: binding.NewUntypedList()}, nil)

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

	before := len(widgetsOfType[*widget.Form](lt.pane.facts))

	// The list drives this binding when a row is clicked.
	require.NotNil(t, lt.selected)
	require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: richGameData, Version: "2.1"}))

	require.Greater(t, len(widgetsOfType[*widget.Form](lt.pane.facts)), before,
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

// The pane opens on the overview: the pictures and a few facts, not a form.
func TestLibraryTab_PaneOpensOnTheOverview(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)

	var titles []string
	for _, item := range lt.pane.tabs.Items {
		titles = append(titles, item.Text)
	}
	require.Equal(t, []string{"Overview", "Details", "Download"}, titles)
	require.Equal(t, "Overview", lt.pane.tabs.Selected().Text)
	require.NotNil(t, lt.gallery, "the details pane carries a place for pictures")
}

// Whichever tab is open, the download button stays where it is.
func TestLibraryTab_DownloadStaysBelowTheTabs(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: richGameData}))

		require.NotNil(t, buttonWithLabel(lt.content, "Download Game"),
			"the download button is part of the pane")

		for _, tab := range lt.pane.tabs.Items {
			require.Nil(t, buttonWithLabel(tab.Content, "Download Game"),
				"the download button must not be inside the %q tab, or it scrolls away with it", tab.Text)
		}
	})
}

// What GOG's store publishes fills in what the local catalogue cannot say.
func TestGameDetails_IncludesStoreInformation(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	updateStatusCache = make(map[int]updateStatus)
	t.Cleanup(func() { updateStatusCache = make(map[int]updateStatus) })

	meta := &client.GameMetadata{
		Developers:  []string{"Santa Monica Studio"},
		Publisher:   "PlayStation PC LLC",
		ReleaseDate: "2024-03-12",
		Genres:      []string{"Action", "Adventure"},
		Features:    []string{"Achievements"},
		AgeRating:   "Mature 17+",
		Voiceovers:  []string{"English", "German"},
		InstalledMB: 40981,
	}
	details := gameDetails(db.Game{ID: 42, Title: "Rich Game", Data: richGameData},
		&DownloadManager{Tasks: binding.NewUntypedList()}, meta)

	require.Equal(t, "Santa Monica Studio", detailValue(t, details, "Developer"))
	require.Equal(t, "PlayStation PC LLC", detailValue(t, details, "Publisher"))
	require.Equal(t, "2024-03-12", detailValue(t, details, "Released"))
	require.Equal(t, "Action, Adventure", detailValue(t, details, "Genres"))
	require.Equal(t, "Mature 17+", detailValue(t, details, "Age rating"))
	require.Equal(t, "English, German", detailValue(t, details, "Voiceovers"))
	require.Equal(t, "40.0 GiB", detailValue(t, details, "Installed size"))
}

// Games delisted from the store keep working; they just say less.
func TestGameDetails_OmitsStoreFactsThatAreMissing(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	details := gameDetails(db.Game{ID: 42, Title: "Rich Game", Data: richGameData},
		&DownloadManager{Tasks: binding.NewUntypedList()}, &client.GameMetadata{})

	for _, label := range []string{"Developer", "Publisher", "Released", "Genres", "Age rating", "Installed size"} {
		for _, detail := range details {
			require.NotEqual(t, label, detail.Label, "%q has nothing to show", label)
		}
	}
}

func TestRenderStoreHeader_ShowsTheDescription(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	rendered := renderStoreHeader(test.NewWindow(nil), "A summary of the game.")

	var texts []string
	for _, label := range widgetsOfType[*widget.Label](rendered) {
		texts = append(texts, label.Text)
	}
	require.Contains(t, texts, "A summary of the game.")
	require.Nil(t, buttonWithLabel(rendered, "More"), "a short description is all there is")
}

// A game GOG no longer describes must not take up the space of one it does.
func TestRenderStoreHeader_IsNothingWithoutADescription(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	require.Nil(t, renderStoreHeader(test.NewWindow(nil), ""))
}

// A description of any length has to leave room for the facts below it, so only
// its opening is shown and the rest is a click away.
func TestRenderStoreHeader_LongDescriptionIsCutWithAWayToReadIt(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	long := strings.Repeat("A long description. ", 40)
	rendered := renderStoreHeader(test.NewWindow(nil), long)

	shown := widgetsOfType[*widget.Label](rendered)[0].Text
	require.Less(t, len(shown), len(long))
	require.LessOrEqual(t, utf8.RuneCountInString(shown), summaryClamp+1, "the opening is what is shown")
	require.True(t, strings.HasSuffix(shown, "\u2026"))
	require.NotNil(t, buttonWithLabel(rendered, "More"), "the rest has to be reachable")
}

func TestClampSummary(t *testing.T) {
	short, more := clampSummary("Two words.")
	require.Equal(t, "Two words.", short)
	require.False(t, more)

	paragraph, more := clampSummary("The game.\nA list of features.")
	require.Equal(t, "The game.", paragraph, "the opening paragraph describes the game")
	require.True(t, more)

	cut, more := clampSummary(strings.Repeat("word ", 200))
	require.True(t, more)
	require.LessOrEqual(t, utf8.RuneCountInString(cut), summaryClamp+1)
	require.NotContains(t, cut, "wor\u2026", "words are not cut in half")

	empty, more := clampSummary("   ")
	require.Empty(t, empty)
	require.False(t, more)
}

// The overview shows the few facts worth seeing at a glance, not all fifteen.
func TestRenderKeyFacts_ShowsOnlyWhatMatters(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	rendered := renderKeyFacts([]gameDetail{
		{"Game ID", "42"}, {"Version", "2.1"}, {"Released", "2024-03-12"},
		{"Genres", "Action"}, {"Platforms", "Windows"}, {"Installed size", "40.0 GiB"},
	})

	texts := labelTexts(rendered)
	require.Contains(t, texts, "Version")
	require.Contains(t, texts, "2.1")
	require.Contains(t, texts, "40.0 GiB")
	require.NotContains(t, texts, "Genres", "the full list is a tab away")
	require.NotContains(t, texts, "Game ID")
}

// The facts are filled twice: once with what gogg holds, once when GOG answers.
func TestFillDetails_AddsTheStoreFactsWhenTheyArrive(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		game := db.Game{ID: 7, Title: "Game 7", Data: richGameData, Version: "2.1"}
		require.NoError(t, lt.selected.Set(game))

		require.NotContains(t, formLabels(lt.pane.facts), "Publisher", "nothing is known about the store yet")

		fillDetails(lt.pane, game, lt.dm, &client.GameMetadata{Publisher: "A Publisher"})
		require.Contains(t, formLabels(lt.pane.facts), "Publisher")
	})
}

// The description replaces whatever the previous game left behind, and a game
// with none leaves the space empty.
func TestFillStoreHeader_ShowsAndClearsTheDescription(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		win := test.NewWindow(nil)

		fillStoreHeader(lt.pane, &client.GameMetadata{
			Summary: "A summary of the game.", StoreURL: "https://www.gog.com/game/x",
		}, win)
		require.NotEmpty(t, lt.pane.storeHeader.Objects)
		require.True(t, buttonWithLabel(lt.content, "View on GOG").Visible(),
			"the way to the store page shows once there is one")

		fillStoreHeader(lt.pane, &client.GameMetadata{}, win)
		require.Empty(t, lt.pane.storeHeader.Objects, "a game GOG does not describe shows nothing")
		require.False(t, buttonWithLabel(lt.content, "View on GOG").Visible())

		fillStoreHeader(lt.pane, nil, win)
		require.Empty(t, lt.pane.storeHeader.Objects)
	})
}

// formLabels names the facts a filled pane is showing.
func formLabels(root fyne.CanvasObject) []string {
	var labels []string
	for _, form := range widgetsOfType[*widget.Form](root) {
		for _, item := range form.Items {
			labels = append(labels, item.Text)
		}
	}
	return labels
}

// The description has to be readable without opening the facts accordion.
func TestLibraryTab_StoreHeaderShowsInThePane(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: richGameData}))

		fillStoreHeader(lt.pane, &client.GameMetadata{
			Summary: "A summary of the game.", StoreURL: "https://www.gog.com/game/x",
		}, test.NewWindow(nil))

		require.NotEmpty(t, lt.pane.storeHeader.Objects, "the description shows on the overview")
		require.True(t, buttonWithLabel(lt.content, "View on GOG").Visible())
	})
}

// A game with a description and a strip of pictures still has to fit in the
// window, or the download button ends up off the screen.
func TestLibraryTab_FitsInADefaultWindowWithStoreInformation(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 3)
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Rich Game", Data: richGameData}))

		fillStoreHeader(lt.pane, &client.GameMetadata{
			Summary:  strings.Repeat("A long description. ", 40),
			StoreURL: "https://www.gog.com/game/x",
		}, test.NewWindow(nil))
		showGallery(lt.gallery, gameWithCover(storeStub(t, nil)+"/cover"),
			shots(storeStub(t, nil), 6))

		// The defaults in window.go.
		const defaultWidth, defaultHeight = 960, 640
		min := lt.content.MinSize()
		t.Logf("library minimum size with store information: %.0fx%.0f", min.Width, min.Height)

		require.LessOrEqual(t, min.Height, float32(defaultHeight))
		require.LessOrEqual(t, min.Width, float32(defaultWidth))
	})
}

// A button handed the whole width of the pane reads as a banner. They keep the
// size they were given, however wide the pane is dragged.
func TestDetailsPane_ButtonsKeepTheirOwnSize(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: richGameData}))
		fillStoreHeader(lt.pane, &client.GameMetadata{
			Summary: "A summary.", StoreURL: "https://www.gog.com/game/x",
		}, test.NewWindow(nil))

		win := test.NewWindow(lt.content)
		t.Cleanup(win.Close)
		win.Resize(fyne.NewSize(1600, 900))

		for name, want := range map[string]fyne.Size{
			"Download Game": paneActionSize,
			"Estimate Size": paneButtonSize,
			"View on GOG":   paneButtonSize,
			"gogdb.org":     paneLinkSize,
		} {
			button := buttonWithLabel(lt.content, name)
			require.NotNil(t, button, "%q is missing from the pane", name)
			require.Equal(t, want.Width, button.Size().Width, "%q is stretched", name)
			require.Equal(t, want.Height, button.Size().Height, "%q is stretched", name)
		}
	})
}

// Fifteen facts in one column is a wall. Given room, they go in two.
func TestRenderGameDetails_SplitsIntoTwoColumnsWhenThereIsRoom(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	details := make([]gameDetail, 0, 8)
	for _, label := range []string{"Game ID", "Version", "Developer", "Publisher", "Released", "Genres", "Platforms", "DLCs"} {
		details = append(details, gameDetail{label, "value"})
	}
	rendered := renderGameDetails(details)

	forms := widgetsOfType[*widget.Form](rendered)
	require.Len(t, forms, 2, "the facts are halved")
	require.Len(t, forms[0].Items, 4)
	require.Len(t, forms[1].Items, 4)

	grid := rendered.(*factsGrid)
	win := test.NewWindow(grid)
	t.Cleanup(win.Close)

	win.Resize(fyne.NewSize(600, 500))
	require.Greater(t, forms[1].Position().X, float32(0), "side by side in a wide pane")
	require.Equal(t, float32(0), forms[1].Position().Y)
	sideBySide := grid.MinSize().Height

	win.Resize(fyne.NewSize(400, 500))
	require.Equal(t, float32(0), forms[1].Position().X, "and stacked in a narrow one")
	require.Greater(t, forms[1].Position().Y, float32(0))

	require.Less(t, sideBySide, grid.MinSize().Height,
		"two columns must claim back the height, not only rearrange it")
}

// A short list is not worth splitting.
func TestRenderGameDetails_LeavesAShortListAlone(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	rendered := renderGameDetails([]gameDetail{{"Game ID", "42"}, {"Version", "2.1"}})
	_, single := rendered.(*widget.Form)
	require.True(t, single)
}

// Grouping the switches must not lose any of them.
func TestDownloadOptions_KeepsEverySwitchUnderAHeading(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)
	options := lt.pane.tabs.Items[2].Content

	var labels []string
	for _, check := range widgetsOfType[*widget.Check](options) {
		labels = append(labels, check.Text)
	}
	require.ElementsMatch(t, []string{
		"Include Extras", "Include DLCs", "Resume Downloads", "Skip Patches",
		"Keep only latest installer", "Flatten Directory", "RomM folder layout (platform/game)",
	}, labels)

	headings := labelTexts(options)
	require.Contains(t, headings, "What to download")
	require.Contains(t, headings, "Which files")
	require.Contains(t, headings, "Where they go")
}
