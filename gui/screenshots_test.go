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

	"fyne.io/fyne/v2/test"
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
func TestShowPicture_FetchesTheLargeRendition(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base, asked := pictureServer(t)

	dir := t.TempDir()
	showPicture(test.NewWindow(nil),
		galleryPicture{ThumbnailURL: base + "/thumb_112.jpg", LargeURL: base + "/large_748.jpg"},
		newCoverCache(dir))

	require.Eventually(t, func() bool { return cachedFiles(dir) == 1 }, 5*time.Second, 20*time.Millisecond)
	require.True(t, asked.contains("_748.jpg"))
	require.False(t, asked.contains("_112.jpg"), "the thumbnail is already on screen")
}
