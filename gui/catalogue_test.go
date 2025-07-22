package gui_test

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/gui"
)

type mockAuthStorer struct{}

func (m *mockAuthStorer) GetTokenRecord() (*db.Token, error)      { return nil, nil }
func (m *mockAuthStorer) UpsertTokenRecord(token *db.Token) error { return nil }

type mockAuthRefresher struct{}

func (m *mockAuthRefresher) PerformTokenRefresh(refreshToken string) (string, string, int64, error) {
	return "", "", 0, nil
}

func initDBForTest(t *testing.T) {
	if err := db.InitDB(); err != nil {
		t.Fatalf("Failed to initialize DB: %v", err)
	}
}

func TestCatalogueListUI(t *testing.T) {
	initDBForTest(t)

	app := test.NewApp()
	t.Cleanup(app.Quit)

	w := app.NewWindow("Test CatalogueListUI")
	t.Cleanup(w.Close)

	co := gui.CatalogueListUI(w)
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

	co := gui.SearchCatalogueUI(w, "Test", false, func() {})
	if co == nil {
		t.Error("SearchCatalogueUI (by title) returned nil, expected a valid fyne.CanvasObject")
	}

	co2 := gui.SearchCatalogueUI(w, "1", true, func() {})
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

	mockService := auth.NewService(&mockAuthStorer{}, &mockAuthRefresher{})

	gui.RefreshCatalogueUI(w, mockService)
}

func TestExportCatalogueUI(t *testing.T) {
	initDBForTest(t)

	app := test.NewApp()
	t.Cleanup(app.Quit)

	w := app.NewWindow("Test ExportCatalogueUI")
	t.Cleanup(w.Close)

	gui.ExportCatalogueUI(w, "csv")
	gui.ExportCatalogueUI(w, "json")
}
