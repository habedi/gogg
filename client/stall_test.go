package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// shortStallWindow shrinks the watchdog for one test and puts it back after.
func shortStallWindow(t *testing.T, d time.Duration) {
	t.Helper()
	old := StallTimeout
	StallTimeout = d
	t.Cleanup(func() { StallTimeout = old })
}

func stallGame(manualURL string) Game {
	return Game{
		Title: "Quiet Game",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &manualURL, Name: "setup.exe"}}},
		}},
	}
}

// A server that goes silent mid-transfer is cut off by the watchdog instead
// of hanging the download until TCP notices, and the cut counts as transient:
// every attempt gets its turn.
func TestDownloadGameFiles_CutsOffAStalledTransfer(t *testing.T) {
	shortStallWindow(t, 50*time.Millisecond)

	var gets atomic.Int32
	body := []byte("the first half arrives, the rest never does")
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/downloads/") {
			// The redirect check gets a real redirect, the way GOG answers,
			// so it stays out of the transfer counts below.
			http.Redirect(w, r, srv.URL+"/cdn/setup.exe", http.StatusFound)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)*2))
		if r.Method == http.MethodHead {
			return
		}
		gets.Add(1)
		_, _ = w.Write(body)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	err := DownloadGameFiles(context.Background(), "token", stallGame(srv.URL+"/downloads/g/f"), t.TempDir(),
		DownloadOptions{Language: "en", Platform: "windows", Resume: true, Threads: 1}, io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), "stalled")
	require.Equal(t, int32(downloadAttempts), gets.Load(), "a stall is transient, so every attempt runs")
}

// After a stall, the retry resumes at the byte where the line went quiet, and
// the finished file carries both halves.
func TestDownloadGameFiles_ResumesAfterAStall(t *testing.T) {
	shortStallWindow(t, 50*time.Millisecond)

	full := []byte("first-half-bytes|second-half-bytes")
	half := len(full) / 2
	var gets atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/downloads/") {
			http.Redirect(w, r, srv.URL+"/cdn/setup.exe", http.StatusFound)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(full)))
			return
		}
		if gets.Add(1) == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(full)))
			_, _ = w.Write(full[:half])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		offset := 0
		if rangeHeader := r.Header.Get("Range"); strings.HasPrefix(rangeHeader, "bytes=") {
			offset, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rangeHeader, "bytes="), "-"))
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(full)-offset))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(full)-1, len(full)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(full[offset:])
	}))
	t.Cleanup(srv.Close)

	tmp := t.TempDir()
	require.NoError(t, DownloadGameFiles(context.Background(), "token", stallGame(srv.URL+"/downloads/g/f"), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Resume: true, Threads: 1}, io.Discard))

	saved, err := os.ReadFile(filepath.Join(tmp, "quiet-game", "windows", "setup.exe"))
	require.NoError(t, err)
	require.Equal(t, full, saved, "the resumed attempt continues, not restarts")
	require.Equal(t, int32(2), gets.Load(), "one stalled attempt, one resumed attempt")
}

// A healthy transfer that merely takes longer than the window in total is
// left alone: the watchdog watches for silence, not for slowness.
func TestDownloadGameFiles_LeavesASlowButLiveTransferAlone(t *testing.T) {
	shortStallWindow(t, 60*time.Millisecond)

	pieces := 5
	piece := []byte("chunk-of-a-live-slow-transfer-")
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/downloads/") {
			http.Redirect(w, r, srv.URL+"/cdn/setup.exe", http.StatusFound)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(piece)*pieces))
		if r.Method == http.MethodHead {
			return
		}
		for i := 0; i < pieces; i++ {
			_, _ = w.Write(piece)
			w.(http.Flusher).Flush()
			time.Sleep(30 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)

	tmp := t.TempDir()
	require.NoError(t, DownloadGameFiles(context.Background(), "token", stallGame(srv.URL+"/downloads/g/f"), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Threads: 1}, io.Discard))

	saved, err := os.ReadFile(filepath.Join(tmp, "quiet-game", "windows", "setup.exe"))
	require.NoError(t, err)
	require.Len(t, saved, len(piece)*pieces)
}
