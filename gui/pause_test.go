package gui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// Pausing keeps what has arrived; Resume picks the transfer up from there and
// finishes the file.
func TestDownload_PauseKeepsTheBytesAndResumeFinishes(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Preferences().SetBool("soundEnabled", false)
	app.Preferences().SetBool(prefNotifications, false)

	body := []byte("0123456789abcdef0123456789abcdef")
	half := len(body) / 2
	gate := make(chan struct{})
	var released atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.WriteHeader(http.StatusOK)
			return
		}
		if !released.Load() {
			// First pass: half the file, then hold the connection until the
			// pause cuts it.
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			_, _ = w.Write(body[:half])
			w.(http.Flusher).Flush()
			<-gate
			return
		}
		// The resumed pass asks for the rest.
		var from int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &from); err == nil && from > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(body)-1, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[from:])
			return
		}
		_, _ = w.Write(body)
	}))
	// Registered after Close, so the gate opens first and Close can finish.
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(gate) })

	root := t.TempDir()
	rawURL := server.URL + "/game/setup.exe"
	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	require.NoError(t, executeDownload(dm, queuedDownload{
		authService: stubAuthService(),
		game: db.Game{ID: 21, Title: "Pausable", Data: `{"title":"Pausable","downloads":[` +
			`["English",{"windows":[{"manualUrl":"` + rawURL + `","name":"setup.exe","size":"32 B"}]}]],` +
			`"extras":[],"dlcs":[]}`},
		downloadPath: root, language: "English", platformName: "windows",
		resumeFlag: true, numThreads: 1,
	}))

	task := dm.tasksSnapshot()[0]
	partPath := filepath.Join(root, "pausable", "windows", "setup.exe.part")
	require.Eventually(t, func() bool {
		info, err := os.Stat(partPath)
		return err == nil && info.Size() >= int64(half)
	}, 5*time.Second, 10*time.Millisecond, "half the file has to arrive before the pause")

	task.pause()
	require.Eventually(t, func() bool { return task.State() == StatePaused },
		5*time.Second, 10*time.Millisecond)

	info, err := os.Stat(partPath)
	require.NoError(t, err, "pausing keeps the partial file")
	require.GreaterOrEqual(t, info.Size(), int64(half))

	// The server serves the rest from here on.
	released.Store(true)
	require.NoError(t, dm.resumePaused(task))

	require.Eventually(t, func() bool {
		tasks := dm.tasksSnapshot()
		return len(tasks) == 1 && tasks[0].State() == StateCompleted
	}, 5*time.Second, 10*time.Millisecond)

	data, err := os.ReadFile(filepath.Join(root, "pausable", "windows", "setup.exe"))
	require.NoError(t, err)
	require.Equal(t, body, data, "the resumed download finishes the same file")
}

// A paused card offers Resume; one restored from history, whose request did
// not survive, says so instead.
func TestDownloadsTab_PausedCardOffersResume(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	paused := &DownloadTask{
		ID: 1, Title: "One", request: queuedDownload{game: db.Game{ID: 1}},
		Status: binding.NewString(), Details: binding.NewString(),
		Progress: binding.NewFloat(), FileStatus: binding.NewString(),
	}
	paused.SetState(StatePaused)
	require.NoError(t, dm.AddTask(paused))

	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	offMain(t, func() {
		tab := DownloadsTabUI(win, dm)
		list := widgetsOfType[*widget.List](tab)[0]

		row := newDownloadRow().(*downloadRow)
		list.UpdateItem(0, row)
		require.Equal(t, "Resume", row.actionBtn.Text, "a paused download offers the way on")
		require.False(t, row.actionBtn.Disabled())
		require.False(t, row.pauseBtn.Visible(), "it is already paused")

		// A running download offers the pause instead.
		running := &DownloadTask{
			ID: 2, Title: "Two", InstanceID: time.Now(), CancelFunc: func() {},
			Status: binding.NewString(), Details: binding.NewString(),
			Progress: binding.NewFloat(), FileStatus: binding.NewString(),
		}
		running.SetState(StateDownloading)
		require.NoError(t, dm.AddTask(running))
		list.UpdateItem(0, row) // running sorts first
		require.True(t, row.pauseBtn.Visible())
		require.Equal(t, "Cancel", row.actionBtn.Text)

		// One paused in an earlier run carries no request: nothing to resume.
		restored := &DownloadTask{
			ID: 3, Title: "Three", InstanceID: time.Now().Add(time.Second),
			Status: binding.NewString(), Details: binding.NewString(),
			Progress: binding.NewFloat(), FileStatus: binding.NewString(),
		}
		restored.SetState(StatePaused)
		require.NoError(t, dm.AddTask(restored))
		list.UpdateItem(1, row)
		require.Equal(t, "Paused", row.actionBtn.Text)
		require.True(t, row.actionBtn.Disabled(), "its request did not survive the restart")
	})
}
