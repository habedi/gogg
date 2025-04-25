package ui_test

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/ui"
)

// initDBForTest initializes the database for UI tests.
// Adjust the function name if your DB package uses a different one.
func initDBForTest(t *testing.T) {
	if err := db.InitDB(); err != nil {
		t.Fatalf("Failed to initialize DB: %v", err)
	}
	// Optionally, insert some dummy data so UI functions have something to show.
	// For example:
	/*
		err := db.PutInGame(1, "Test Game", `{"dummy":"data"}`)
		if err != nil {
			t.Fatalf("Failed to insert test game: %v", err)
		}
	*/
}

func TestCatalogueListUI(t *testing.T) {
	initDBForTest(t)

	// Create a temporary Fyne app and window.
	app := test.NewApp()
	t.Cleanup(app.Quit)

	w := app.NewWindow("Test CatalogueListUI")
	t.Cleanup(w.Close)

	// Call CatalogueListUI and verify it returns a non-nil CanvasObject.
	co := ui.CatalogueListUI(w)
	if co == nil {
		t.Error("CatalogueListUI returned nil, expected a valid fyne.CanvasObject")
	}
}

func TestSearchCatalogueUI(t *testing.T) {
	initDBForTest(t)

	app := test.NewApp()
	t.Cleanup(app.Quit)

	w := app.NewWindow("Test SearchCatalogueUI")
	t.Cleanup(w.Close)

	// Test search by title.
	co := ui.SearchCatalogueUI(w, "Test", false)
	if co == nil {
		t.Error("SearchCatalogueUI (by title) returned nil, expected a valid fyne.CanvasObject")
	}

	// Test search by ID (assuming "1" is valid).
	co2 := ui.SearchCatalogueUI(w, "1", true)
	if co2 == nil {
		t.Error("SearchCatalogueUI (by ID) returned nil, expected a valid fyne.CanvasObject")
	}
}

func TestRefreshCatalogueUI(t *testing.T) {
	initDBForTest(t)

	app := test.NewApp()
	t.Cleanup(app.Quit)

	w := app.NewWindow("Test RefreshCatalogueUI")
	t.Cleanup(w.Close)

	// Call RefreshCatalogueUI; we simply check that it runs without panic.
	// (This function opens a dialog and runs asynchronously.)
	ui.RefreshCatalogueUI(w)
	// In a real test you might wait for completion or verify side effects.
}

func TestExportCatalogueUI(t *testing.T) {
	initDBForTest(t)

	app := test.NewApp()
	t.Cleanup(app.Quit)

	w := app.NewWindow("Test ExportCatalogueUI")
	t.Cleanup(w.Close)

	// Call ExportCatalogueUI for both supported formats.
	// Since the export function opens a file save dialog,
	// we only verify that the call does not panic.
	ui.ExportCatalogueUI(w, "csv")
	ui.ExportCatalogueUI(w, "json")
}
