package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// testCoverCache is a cover cache whose fetches are cancelled and waited out
// when the test ends. A delivery that outlives its test lands in widgets the
// next test is using; closing the cache is what makes the end of the test
// mean the end of its background work.
func testCoverCache(t *testing.T, dir string) *coverCache {
	t.Helper()
	cache := newCoverCache(dir)
	t.Cleanup(cache.close)
	return cache
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
	}, coverBanner)

	require.Equal(t, "https://images-1.gog-statics.com/aaa"+bannerRendition, source.URL)
	require.False(t, source.Faded, "the banner needs no cropping")
}

// Catalogues refreshed before gogg recorded banners still show something.
func TestCoverSourceFor_FallsBackToTheBackgroundPicture(t *testing.T) {
	source := coverSourceFor(db.Game{
		ID: 1, Title: "One", Data: gameDataWithCover("//images-2.gog-statics.com/bbb"),
	}, coverBanner)

	require.Equal(t, "https://images-2.gog-statics.com/bbb"+backgroundRendition, source.URL)
	require.True(t, source.Faded, "GOG fades that one, so it has to be cropped")
}

// An address that already names a file is left alone.
func TestCoverSourceFor_LeavesAnExplicitImageAlone(t *testing.T) {
	source := coverSourceFor(db.Game{ID: 1, CoverImage: "https://images.gog.com/abc_bg.jpg"}, coverBanner)
	require.Equal(t, "https://images.gog.com/abc_bg.jpg", source.URL)
}

func TestCoverSourceFor_EmptyWhenThereIsNoArtwork(t *testing.T) {
	require.Empty(t, coverSourceFor(db.Game{ID: 1, Data: `{"title":"G"}`}, coverBanner).URL)
	require.Empty(t, coverSourceFor(db.Game{ID: 1, Data: "{not json"}, coverBanner).URL)
}

// Covers are fetched once and kept, so scrolling the library does not hammer
// GOG's image servers on every pass.
func TestCoverCache_FetchesOnceAndReusesTheFile(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	srv, hits := imageServer(t)

	cache := testCoverCache(t, t.TempDir())
	game := db.Game{ID: 1, Title: "One", Data: gameDataWithCover(srv.URL + "/bg.jpg")}

	first, _, err := cache.fetch(game, coverBanner)
	require.NoError(t, err)
	require.NotNil(t, first)

	second, _, err := cache.fetch(game, coverBanner)
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

	_, _, err := testCoverCache(t, dir).fetch(game, coverBanner)
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the cover is written for the next run")

	// A cache built afresh, as after a restart, reuses what is already there.
	served, _, err := testCoverCache(t, dir).fetch(game, coverBanner)
	require.NoError(t, err)
	require.NotNil(t, served)
}

// One URL can be fetched twice at once, as it is when a picture serves as both
// the thumbnail and the large one. Each fetch must write a file of its own, or
// the two writers meet on one temp file and, on Windows, neither picture is
// cached.
func TestCoverCache_ConcurrentFetchesOfOneURLLeaveOneCover(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	// The requests are held until all of them have arrived, so the writes
	// overlap the way they do behind a gallery.
	const fetchers = 4
	arrived := make(chan struct{}, fetchers)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		arrived <- struct{}{}
		<-release
		_, _ = w.Write(onePixelPNG)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cache := testCoverCache(t, dir)

	done := make(chan error, fetchers)
	for i := 0; i < fetchers; i++ {
		go func() {
			_, err := cache.fetchURL(srv.URL + "/one.jpg")
			done <- err
		}()
	}
	for i := 0; i < fetchers; i++ {
		<-arrived
	}
	close(release)
	for i := 0; i < fetchers; i++ {
		require.NoError(t, <-done)
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	require.Len(t, names, 1, "one cover is cached and no temp file is left behind: %v", names)
	require.False(t, strings.HasSuffix(names[0], ".part"), "the cover is renamed into place")
}

// Two writers for one cover must not meet on the same temp file. Windows
// refuses to rename a file another writer still holds open, and the cleanup
// after that failure takes away the file the other writer was about to
// rename, so a shared name costs both of them their picture.
func TestWriteCoverTemp_GivesEachWriterItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cover.img")

	first, err := writeCoverTemp(dir, target, []byte("one"))
	require.NoError(t, err)
	second, err := writeCoverTemp(dir, target, []byte("two"))
	require.NoError(t, err)

	require.NotEqual(t, first, second, "two writers for one cover get two files")
	require.FileExists(t, first)
	require.FileExists(t, second)

	data, err := os.ReadFile(first)
	require.NoError(t, err)
	require.Equal(t, "one", string(data), "neither writer overwrites what the other wrote")
}

func TestCoverCache_ReportsGamesWithNoCover(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	_, _, err := testCoverCache(t, t.TempDir()).fetch(db.Game{ID: 1, Title: "One", Data: "{}"}, coverBanner)
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
	_, _, err := testCoverCache(t, dir).fetch(db.Game{ID: 1, Title: "One", Data: gameDataWithCover(srv.URL + "/bg.jpg")}, coverBanner)
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

	cache := testCoverCache(t, t.TempDir())
	game := db.Game{ID: 1, Title: "One", Data: gameDataWithCover(srv.URL + "/bg.jpg")}

	var delivered atomic.Int64
	cache.load(game, coverBanner, func(int) bool { return true }, func([]byte, coverSource) { delivered.Add(1) })
	require.Eventually(t, func() bool { return delivered.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

	// A cell that has moved on refuses the answer.
	var stale atomic.Int64
	cache.load(game, coverBanner, func(int) bool { return false }, func([]byte, coverSource) { stale.Add(1) })
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

// A list row wants a thumbnail, not a banner: 1.5 KB against 27 KB.
func TestCoverSourceFor_ThumbnailIsSmallerThanTheBanner(t *testing.T) {
	game := db.Game{ID: 1, CoverImage: "//images-1.gog-statics.com/aaa"}

	require.Equal(t, "https://images-1.gog-statics.com/aaa"+thumbnailRendition,
		coverSourceFor(game, coverThumbnail).URL)
	require.Equal(t, "https://images-1.gog-statics.com/aaa"+bannerRendition,
		coverSourceFor(game, coverBanner).URL)
}
