package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// pictureServer serves a picture for anything asked of it and records the paths.
func pictureServer(t *testing.T) (string, *pathLog) {
	t.Helper()
	asked := &pathLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.add(r.URL.Path)
		_, _ = w.Write(onePixelPNG)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, asked
}

type pathLog struct {
	count atomic.Int64
	paths atomic.Value // []string, replaced whole so readers never see a partial slice
}

func (l *pathLog) add(path string) {
	previous, _ := l.paths.Load().([]string)
	l.paths.Store(append(append([]string{}, previous...), path))
	l.count.Add(1)
}

func (l *pathLog) contains(substring string) bool {
	paths, _ := l.paths.Load().([]string)
	for _, path := range paths {
		if strings.Contains(path, substring) {
			return true
		}
	}
	return false
}

// cachedFiles counts what a cache has written. Tests wait on it so a picture
// cannot land in the directory after the test that owns it has been cleaned up.
func cachedFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	return len(entries)
}

func shots(base string, n int) []client.Screenshot {
	made := make([]client.Screenshot, 0, n)
	for i := 0; i < n; i++ {
		made = append(made, client.Screenshot{
			ThumbnailURL: base + "/thumb" + string(rune('a'+i)) + "_112.jpg",
			LargeURL:     base + "/large" + string(rune('a'+i)) + "_748.jpg",
		})
	}
	return made
}

// The strip shows every picture the store page has.
func TestScreenshotStrip_ShowsOneThumbnailPerScreenshot(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base, asked := pictureServer(t)

	dir := t.TempDir()
	strip := screenshotStrip(shots(base, 3), newCoverCache(dir),
		func() bool { return true }, func(client.Screenshot) {})

	require.Len(t, widgetsOfType[*screenshotThumb](strip), 3)
	require.Eventually(t, func() bool { return cachedFiles(dir) == 3 }, 5*time.Second, 20*time.Millisecond)
	require.Equal(t, int64(3), asked.count.Load())
	require.True(t, asked.contains("_112.jpg"), "the strip must ask for thumbnails, not full pictures")
	require.False(t, asked.contains("_748.jpg"), "the large rendition costs 45 times as much")
}

// Games with no pictures must not leave an empty strip behind.
func TestScreenshotStrip_IsNothingWithoutScreenshots(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	require.Nil(t, screenshotStrip(nil, newCoverCache(t.TempDir()),
		func() bool { return true }, func(client.Screenshot) {}))
}

// Tapping a thumbnail opens the picture, and only then is the large rendition
// worth fetching.
func TestScreenshotStrip_TapOpensTheLargeRendition(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	var opened client.Screenshot
	strip := screenshotStrip(shots(base, 2), newCoverCache(t.TempDir()),
		func() bool { return true }, func(shot client.Screenshot) { opened = shot })

	thumbs := widgetsOfType[*screenshotThumb](strip)
	require.Len(t, thumbs, 2)
	test.Tap(thumbs[1])

	require.Equal(t, base+"/largeb_748.jpg", opened.LargeURL)
}

// Selecting another game before the pictures arrive must not fill the strip of
// the game now on screen.
func TestScreenshotStrip_DropsPicturesForAGameNoLongerShown(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base, _ := pictureServer(t)

	dir := t.TempDir()
	strip := screenshotStrip(shots(base, 1), newCoverCache(dir),
		func() bool { return false }, func(client.Screenshot) {})

	require.Eventually(t, func() bool { return cachedFiles(dir) == 1 }, 5*time.Second, 20*time.Millisecond)
	for _, thumb := range widgetsOfType[*screenshotThumb](strip) {
		require.Nil(t, thumb.picture.Resource, "a picture for another game must not be shown")
	}
}

// The strip has to be part of the details pane, not a container on its own.
func TestLibraryTab_ScreenshotsShowInTheDetailsPane(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: richGameData}))

		fillScreenshots(lt.screenshots, shots(base, 2), newCoverCache(t.TempDir()),
			test.NewWindow(nil), func() bool { return true })

		require.Len(t, widgetsOfType[*screenshotThumb](lt.content), 2,
			"the pictures must be reachable from the pane the user is looking at")
	})
}

// Selecting another game drops the pictures of the previous one at once, rather
// than leaving them until the new ones arrive.
func TestLibraryTab_ScreenshotsGoWhenTheSelectionChanges(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: richGameData}))
		fillScreenshots(lt.screenshots, shots(base, 2), newCoverCache(t.TempDir()),
			test.NewWindow(nil), func() bool { return true })
		require.NotEmpty(t, widgetsOfType[*screenshotThumb](lt.content))

		require.NoError(t, lt.selected.Set(db.Game{ID: 2, Title: "Game 2", Data: richGameData}))
		require.Empty(t, widgetsOfType[*screenshotThumb](lt.content))
	})
}

// offMain runs a test body on a goroutine of its own. Fyne only queues a
// binding listener when the caller is the main goroutine, so a test that sets a
// binding and then looks at what changed has to run off it to see the change
// rather than race with it.
func offMain(t *testing.T, body func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		body()
	}()
	<-done
}
