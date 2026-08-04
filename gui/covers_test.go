package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// onePixelPNG is the smallest thing a decoder will accept.
var onePixelPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
	0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
	0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
	0x0d, 0x0a, 0x2d, 0xb4,
	0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

func imageServer(t *testing.T) (*httptest.Server, func() int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(onePixelPNG)
	}))
	t.Cleanup(srv.Close)
	return srv, hits.Load
}

// The artwork GOG lists for owned games is preferred: it has no fade baked in,
// so nothing has to be cropped off it.
func TestCoverSourceFor_PrefersTheRecordedBanner(t *testing.T) {
	source := coverSourceFor(db.Game{
		ID: 1, Title: "One", CoverImage: "//images-1.gog-statics.com/aaa",
		Data: gameDataWithCover("//images-2.gog-statics.com/bbb"),
	})

	require.Equal(t, "https://images-1.gog-statics.com/aaa"+bannerRendition, source.URL)
	require.False(t, source.Faded, "the banner needs no cropping")
}

// Catalogues refreshed before gogg recorded banners still show something.
func TestCoverSourceFor_FallsBackToTheBackgroundPicture(t *testing.T) {
	source := coverSourceFor(db.Game{
		ID: 1, Title: "One", Data: gameDataWithCover("//images-2.gog-statics.com/bbb"),
	})

	require.Equal(t, "https://images-2.gog-statics.com/bbb"+backgroundRendition, source.URL)
	require.True(t, source.Faded, "GOG fades that one, so it has to be cropped")
}

// An address that already names a file is left alone.
func TestCoverSourceFor_LeavesAnExplicitImageAlone(t *testing.T) {
	source := coverSourceFor(db.Game{ID: 1, CoverImage: "https://images.gog.com/abc_bg.jpg"})
	require.Equal(t, "https://images.gog.com/abc_bg.jpg", source.URL)
}

func TestCoverSourceFor_EmptyWhenThereIsNoArtwork(t *testing.T) {
	require.Empty(t, coverSourceFor(db.Game{ID: 1, Data: `{"title":"G"}`}).URL)
	require.Empty(t, coverSourceFor(db.Game{ID: 1, Data: "{not json"}).URL)
}

// Covers are fetched once and kept, so scrolling the library does not hammer
// GOG's image servers on every pass.
func TestCoverCache_FetchesOnceAndReusesTheFile(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	srv, hits := imageServer(t)

	cache := newCoverCache(t.TempDir())
	game := db.Game{ID: 1, Title: "One", Data: gameDataWithCover(srv.URL + "/bg.jpg")}

	first, _, err := cache.fetch(game)
	require.NoError(t, err)
	require.NotNil(t, first)

	second, _, err := cache.fetch(game)
	require.NoError(t, err)
	require.NotNil(t, second)

	require.Equal(t, int64(1), hits(), "the second request must be served from disk")
}

func TestCoverCache_KeepsCoversOnDisk(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	srv, _ := imageServer(t)

	dir := t.TempDir()
	game := db.Game{ID: 1, Title: "One", Data: gameDataWithCover(srv.URL + "/bg.jpg")}

	_, _, err := newCoverCache(dir).fetch(game)
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the cover is written for the next run")

	// A cache built afresh, as after a restart, reuses what is already there.
	served, _, err := newCoverCache(dir).fetch(game)
	require.NoError(t, err)
	require.NotNil(t, served)
}

func TestCoverCache_ReportsGamesWithNoCover(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	_, _, err := newCoverCache(t.TempDir()).fetch(db.Game{ID: 1, Title: "One", Data: "{}"})
	require.Error(t, err)
}

func TestCoverCache_ReportsAFailedFetch(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	_, _, err := newCoverCache(dir).fetch(db.Game{ID: 1, Title: "One", Data: gameDataWithCover(srv.URL + "/bg.jpg")})
	require.Error(t, err)

	entries, _ := os.ReadDir(dir)
	require.Empty(t, entries, "a failed fetch must not leave a broken file behind")
}

// The grid asks for covers while scrolling; the answer arrives later and must
// reach the cell that is showing that game by then, not the one that asked.
func TestCoverCache_LoadDeliversToTheCurrentGame(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	srv, _ := imageServer(t)

	cache := newCoverCache(t.TempDir())
	game := db.Game{ID: 1, Title: "One", Data: gameDataWithCover(srv.URL + "/bg.jpg")}

	var delivered atomic.Int64
	cache.load(game, func(int) bool { return true }, func([]byte, coverSource) { delivered.Add(1) })
	require.Eventually(t, func() bool { return delivered.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

	// A cell that has moved on refuses the answer.
	var stale atomic.Int64
	cache.load(game, func(int) bool { return false }, func([]byte, coverSource) { stale.Add(1) })
	require.Never(t, func() bool { return stale.Load() > 0 }, 300*time.Millisecond, 20*time.Millisecond)
}

func gameDataWithCover(url string) string {
	return `{"title":"G","backgroundImage":"` + url + `","downloads":[],"extras":[],"dlcs":[]}`
}

func TestCoverCacheDir(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dir := coverCacheDir()
	require.NotEmpty(t, dir)
	require.Equal(t, "covers", filepath.Base(dir))
}
