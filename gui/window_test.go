package gui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// Everything else here builds one part of the window. This builds the whole of
// it and walks what a user does on a first run, because the parts have been
// wrong together while each was right on its own.
func TestMainContent_BuildsAndSurvivesTheUsualPath(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })
	require.NoError(t, db.UpsertTokenRecord(&db.Token{
		AccessToken: "a", RefreshToken: "r", ExpiresAt: "2999-01-01T00:00:00Z",
	}))
	require.NoError(t, db.PutInGame(1, "Rich Game", richGameData))
	require.NoError(t, db.PutInGame(2, "Another Game", richGameData))
	t.Setenv("GOGG_API_BASE", storeStub(t, nil))

	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	offMain(t, func() {
		content := buildMainContent(win, "1.2.3", nil, NewDownloadManager(), nil)
		win.SetContent(content.tabs)
		win.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))

		// Every tab draws, and none of them forces the window wider than it opens.
		require.Len(t, content.tabs.Items, 5)
		for i, tab := range content.tabs.Items {
			content.tabs.SelectIndex(i)
			require.NotNil(t, tab.Content, "%s has nothing in it", tab.Text)
			require.LessOrEqual(t, tab.Content.MinSize().Width, float32(defaultWindowWidth),
				"%s does not fit the window gogg opens at", tab.Text)
		}
		content.tabs.SelectIndex(0)

		// Picking a game fills the pane beside the list.
		require.NoError(t, content.library.selected.Set(db.Game{
			ID: 1, Title: "Rich Game", Data: richGameData,
		}))
		require.Equal(t, "Rich Game", content.library.pane.title.Text)

		// The dialogs the toolbar hangs off open and close again.
		for _, label := range []string{"Filters", "Export"} {
			button := buttonWithLabel(content.library.content, label)
			require.NotNil(t, button, "the toolbar has no %s", label)
			test.Tap(button)
			require.NotNil(t, topOverlay(t), "%s opened nothing", label)
			win.Canvas().Overlays().Remove(win.Canvas().Overlays().Top())
		}

		// And the search still narrows the list underneath it all.
		content.library.searchEntry.SetText("Another")
		require.Len(t, content.library.listed(), 1)
	})
}

// The window remembers what it was showing, and what it opens on has to be one
// of the tabs it has.
func TestMainContent_OpensOnARealTab(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	tabs := container.NewAppTabs(
		container.NewTabItem("One", widget.NewLabel("one")),
		container.NewTabItem("Two", widget.NewLabel("two")),
	)
	for _, remembered := range []int{-1, 0, 1, 99} {
		require.NotPanics(t, func() { selectRememberedTab(tabs, remembered) },
			"tab %d", remembered)
	}
	selectRememberedTab(tabs, 99)
	require.Equal(t, 0, tabs.SelectedIndex(), "a tab that is no longer there opens the first one")
}
