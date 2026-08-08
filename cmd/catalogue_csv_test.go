package cmd

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/habedi/gogg/db"
)

// Titles containing a double quote must stay parseable as CSV.
func TestExportCatalogueToCSV_EscapesQuotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalogue.csv")
	games := []db.Game{
		{ID: 1, Title: `Sam & Max: "Hit the Road"`},
		{ID: 2, Title: "Plain Title"},
	}

	if err := exportCatalogueToCSV(path, games); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatalf("exported file is not valid CSV: %v\n%s", err, data)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records (incl. header), want 3: %q", len(records), records)
	}
	if records[1][1] != `Sam & Max: "Hit the Road"` {
		t.Errorf("title round-tripped as %q", records[1][1])
	}
	if records[2][1] != "Plain Title" {
		t.Errorf("title round-tripped as %q", records[2][1])
	}
}

// The on-disk shape must not change for titles that need no escaping,
// so existing consumers of the export keep working.
func TestExportCatalogueToCSV_FormatUnchangedForPlainTitles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalogue.csv")
	if err := exportCatalogueToCSV(path, []db.Game{{ID: 42, Title: "Plain Title"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "ID,Title\n42,\"Plain Title\"\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", data, want)
	}
}
