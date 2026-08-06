package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
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

// The facts keep a tab of their own, and the overview carries what a download
// would be made of, so the options and the button that uses them are in the
// same place.
func TestDetailsPane_HasOverviewAndDetailsTabs(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Rich Game", Data: richGameData}))

		var titles []string
		for _, item := range lt.pane.tabs.Items {
			titles = append(titles, item.Text)
		}
		require.Equal(t, []string{"Overview", "Details"}, titles)
		require.Equal(t, "Overview", lt.pane.tabs.Selected().Text)

		overview := lt.pane.tabs.Items[0].Content
		details := lt.pane.tabs.Items[1].Content

		// What a download is made of is with the pictures.
		require.NotNil(t, downloadPathEntry(t, overview))
		require.NotEmpty(t, widgetsOfType[*widget.Check](overview), "and the switches with it")

		// The facts are a tab along, and said once.
		require.Contains(t, formLabels(details), "Version")
		require.NotContains(t, formLabels(overview), "Version")
		require.Equal(t, 1, timesIn(formLabels(lt.pane.content), "Version"))
	})
}

// The buttons stay put while whichever tab is open scrolls under them.
func TestDetailsPane_TheDownloadButtonDoesNotScrollAway(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: richGameData}))

		require.NotNil(t, buttonWithLabel(lt.content, "Download Game"),
			"the download button is part of the pane")
		for _, tab := range lt.pane.tabs.Items {
			require.Nil(t, buttonWithLabel(tab.Content, "Download Game"),
				"but not part of %q, or it scrolls away with it", tab.Text)
		}
	})
}

func timesIn(values []string, want string) int {
	seen := 0
	for _, value := range values {
		if value == want {
			seen++
		}
	}
	return seen
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

	rendered := renderStoreHeader("A summary of the game.")

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

	require.Nil(t, renderStoreHeader(""))
}

// The description is what the store says about a game, and the pane it sits in
// scrolls. Cutting it at 220 characters and hiding the rest behind a button
// made reading it a chore.
func TestRenderStoreHeader_ShowsTheWholeDescription(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	long := strings.Repeat("A long description. ", 40) + "\nAnd a list of features."
	rendered := renderStoreHeader(long)

	shown := widgetsOfType[*widget.Label](rendered)[0].Text
	require.Equal(t, strings.TrimSpace(long), shown, "all of it, not its opening")
	require.Nil(t, buttonWithLabel(rendered, "More"), "there is nothing left to open")
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

		fillStoreHeader(lt.pane, &client.GameMetadata{
			Summary: "A summary of the game.", StoreURL: "https://www.gog.com/game/x",
		})
		require.NotEmpty(t, lt.pane.storeHeader.Objects)
		require.True(t, buttonWithLabel(lt.content, "View on GOG").Visible(),
			"the way to the store page shows once there is one")

		fillStoreHeader(lt.pane, &client.GameMetadata{})
		require.Empty(t, lt.pane.storeHeader.Objects, "a game GOG does not describe shows nothing")
		require.False(t, buttonWithLabel(lt.content, "View on GOG").Visible())

		fillStoreHeader(lt.pane, nil)
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
		})

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
		})
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
		})

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
	options := lt.pane.tabs.Items[0].Content

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

// The pane gives its buttons a size of their own so they line up down the
// right-hand side. That size is a floor rather than a cap: a longer label, or a
// larger font chosen in Settings, must not be cut off.
func TestFixedSize_GrowsForALabelThatWouldBeCut(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	for _, textSize := range []float32{14, 18} {
		app.Settings().SetTheme(&GoggTheme{Theme: theme.DefaultTheme(), textSize: textSize})

		button := widget.NewButtonWithIcon("Download Selected (12)", theme.DownloadIcon(), nil)
		boxed := fixedSize(button, paneActionSize)

		require.GreaterOrEqual(t, boxed.MinSize().Width, button.MinSize().Width,
			"the label is cut off at text size %.0f", textSize)
		require.GreaterOrEqual(t, boxed.MinSize().Height, button.MinSize().Height,
			"the button is cut off at text size %.0f", textSize)
	}
}

// A label that fits leaves the button the size it was given, so the buttons
// beside it still line up.
func TestFixedSize_KeepsTheSizeItWasGiven(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	boxed := fixedSize(widget.NewButton("OK", nil), paneButtonSize)

	require.Equal(t, paneButtonSize, boxed.MinSize())
}

// The download path the user types is what they see next time. It was written
// to one preference and read back from another, so a path that was typed but
// never downloaded to was forgotten.
func TestDownloadForm_RemembersTheTypedPath(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	prefs := app.Preferences()
	prefs.SetString("downloadForm.path", "/typed/by/hand")
	prefs.SetString("lastUsedDownloadPath", "/where/the/last/download/went")

	form := newDownloadFormForTest(t)

	require.Equal(t, "/typed/by/hand", downloadPathEntry(t, form.options).Text)
}

// A catalogue from a version that only ever stored the last download path still
// opens on it.
func TestDownloadForm_FallsBackToTheLastDownloadPath(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	prefs := app.Preferences()
	prefs.RemoveValue("downloadForm.path")
	prefs.SetString("lastUsedDownloadPath", "/where/the/last/download/went")

	form := newDownloadFormForTest(t)

	require.Equal(t, "/where/the/last/download/went", downloadPathEntry(t, form.options).Text)
}

// newDownloadFormForTest builds the download options on their own, away from a
// library fixture that has a download path of its own.
func newDownloadFormForTest(t *testing.T) *downloadForm {
	t.Helper()
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)
	return createDownloadForm(win, nil, &DownloadManager{Tasks: binding.NewUntypedList()},
		binding.NewUntyped(), newGameSelection(), func() []db.Game { return nil })
}

// downloadPathEntry is the box the download path is typed into.
func downloadPathEntry(t *testing.T, root fyne.CanvasObject) *widget.Entry {
	t.Helper()
	for _, entry := range widgetsOfType[*widget.Entry](root) {
		if entry.PlaceHolder == "Enter download path" {
			return entry
		}
	}
	t.Fatal("no download path box")
	return nil
}

// A filter that cannot be read has to say so. It was dropped in silence,
// leaving a search box full of terms that were doing nothing.
func TestLibraryTab_MarksASearchItCannotRead(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	lt, _ := newLibraryFixture(t, 2)

	lt.searchEntry.SetText("size:>=abc")
	require.Error(t, lt.searchEntry.Validate(), "a size that is not a size has to be marked")

	lt.searchEntry.SetText("size:>=10gb")
	require.NoError(t, lt.searchEntry.Validate(), "a filter that reads has to be left alone")

	lt.searchEntry.SetText("god of war")
	require.NoError(t, lt.searchEntry.Validate(), "plain words are not a mistake")
}
