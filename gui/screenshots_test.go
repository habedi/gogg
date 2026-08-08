package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
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
	// Pictures are fetched several at a time, so the paths need a lock of their
	// own: appending to a slice held in an atomic loses whichever of two
	// concurrent writers stored first.
	mu    sync.Mutex
	paths []string
}

func (l *pathLog) add(path string) {
	l.mu.Lock()
	l.paths = append(l.paths, path)
	l.mu.Unlock()
	// Counted last, so a count that has reached its target means every path
	// behind it has been recorded.
	l.count.Add(1)
}

func (l *pathLog) contains(substring string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, path := range l.paths {
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

// Opening a picture on its own fetches the rendition GOG serves large.
func TestShowPictures_FetchesTheLargeRendition(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base, asked := pictureServer(t)

	dir := t.TempDir()
	showPictures(test.NewWindow(nil), []galleryPicture{
		{ThumbnailURL: base + "/thumb_112.jpg", LargeURL: base + "/large_748.jpg"},
	}, 0, testCoverCache(t, dir))

	require.Eventually(t, func() bool { return cachedFiles(dir) == 1 }, 5*time.Second, 20*time.Millisecond)
	require.True(t, asked.contains("_748.jpg"))
	require.False(t, asked.contains("_112.jpg"), "the thumbnail is already on screen")
}

// picturesFor builds a gallery's worth of pictures pointing at the server.
func picturesFor(base string, n int) []galleryPicture {
	made := make([]galleryPicture, 0, n)
	for i := 0; i < n; i++ {
		made = append(made, galleryPicture{
			ThumbnailURL: base + "/thumb" + string(rune('a'+i)) + "_112.jpg",
			LargeURL:     base + "/large" + string(rune('a'+i)) + "_748.jpg",
		})
	}
	return made
}

// gatedPictureServer records what is asked of it at once but answers nothing
// until the test is over. What the dialog fetches is observable without a
// picture ever landing in a widget mid-test, on a goroutine of its own.
func gatedPictureServer(t *testing.T) (string, *pathLog) {
	t.Helper()
	asked := &pathLog{}
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.add(r.URL.Path)
		<-gate
		_, _ = w.Write(onePixelPNG)
	}))
	t.Cleanup(srv.Close)
	// Registered after Close, so the gate opens first and Close can finish.
	t.Cleanup(func() { close(gate) })
	return srv.URL, asked
}

// The dialog moves through the pictures without closing: the arrows either
// side, and the arrow keys, both step along; the ends hold rather than wrap.
func TestShowPictures_MovesLeftAndRight(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base, asked := gatedPictureServer(t)
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	dir := t.TempDir()
	showPictures(win, picturesFor(base, 3), 1, testCoverCache(t, dir))

	overlay := win.Canvas().Overlays().Top()
	require.NotNil(t, overlay)
	viewers := widgetsOfType[*pictureViewer](overlay)
	require.Len(t, viewers, 1, "the dialog body takes the keys")
	viewer := viewers[0]

	counter := func() string {
		for _, label := range labelTexts(overlay) {
			if strings.Contains(label, "/") {
				return label
			}
		}
		return ""
	}
	require.Equal(t, "2 / 3", counter(), "the dialog opens on the picture that was tapped")

	viewer.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	require.Equal(t, "3 / 3", counter())
	// The middle and its neighbours load as they are looked at.
	require.Eventually(t, func() bool { return asked.contains("largec_748.jpg") },
		5*time.Second, 20*time.Millisecond)

	viewer.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	require.Equal(t, "3 / 3", counter(), "the end holds rather than wraps")

	viewer.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	viewer.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	require.Equal(t, "1 / 3", counter())
	viewer.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	require.Equal(t, "1 / 3", counter(), "the start holds too")
}

// A picture on show can be kept: Save writes the bytes GOG served, once they
// have landed.
func TestShowPictures_OffersSave(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base, _ := gatedPictureServer(t)
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	showPictures(win, picturesFor(base, 2), 0, testCoverCache(t, t.TempDir()))

	overlay := win.Canvas().Overlays().Top()
	require.NotNil(t, overlay)

	save := buttonWithLabel(overlay, "Save...")
	require.NotNil(t, save, "the picture can be kept")
	require.True(t, save.Disabled(), "there is nothing to save until the picture lands")
}

func TestSuggestedPictureName(t *testing.T) {
	require.Equal(t, "shot_748.jpg", suggestedPictureName("https://images.gog.com/abc/shot_748.jpg?namespace=x"))
	require.Equal(t, "picture.jpg", suggestedPictureName(""), "a nameless address still saves as something")
}

// One picture has nowhere to go, so the ways to go are not offered.
func TestShowPictures_OnePictureOffersNoTravel(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base, _ := pictureServer(t)
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	showPictures(win, picturesFor(base, 1), 0, testCoverCache(t, t.TempDir()))

	overlay := win.Canvas().Overlays().Top()
	require.NotNil(t, overlay)
	for _, label := range widgetsOfType[*widget.Label](overlay) {
		if strings.Contains(label.Text, " / ") {
			require.False(t, label.Visible(), "a counter with one page is noise")
		}
	}
}
