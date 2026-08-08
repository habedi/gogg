package client

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// smallParallelSizes shrinks the split thresholds for one test and puts them
// back after, so a few kilobytes behave like a few gigabytes.
func smallParallelSizes(t *testing.T, minSize, chunkSize int64) {
	t.Helper()
	oldMin, oldChunk := parallelMinSize, parallelChunkSize
	parallelMinSize, parallelChunkSize = minSize, chunkSize
	t.Cleanup(func() { parallelMinSize, parallelChunkSize = oldMin, oldChunk })
}

// rangeServer serves one file the way GOG's CDN does: a redirect from the
// manual URL, honest HEAD lengths, and 206 answers to range requests. It
// remembers every range it served.
type rangeServer struct {
	srv          *httptest.Server
	body         []byte
	mu           sync.Mutex
	rangesServed []string
	ignoreRanges bool
}

func newRangeServer(t *testing.T, body []byte) *rangeServer {
	t.Helper()
	rs := &rangeServer{body: body}
	rs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/downloads/") {
			http.Redirect(w, r, rs.srv.URL+"/cdn/setup.exe", http.StatusFound)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(rs.body)))
			return
		}
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" || rs.ignoreRanges {
			w.Header().Set("Content-Length", strconv.Itoa(len(rs.body)))
			_, _ = w.Write(rs.body)
			return
		}
		var start, end int
		if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
			// A resume-style open range: bytes=N-
			_, _ = fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
			end = len(rs.body) - 1
		}
		if end > len(rs.body)-1 {
			end = len(rs.body) - 1
		}
		rs.mu.Lock()
		rs.rangesServed = append(rs.rangesServed, rangeHeader)
		rs.mu.Unlock()
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(rs.body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(rs.body[start : end+1])
	}))
	t.Cleanup(rs.srv.Close)
	return rs
}

func (rs *rangeServer) served() []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]string{}, rs.rangesServed...)
}

func (rs *rangeServer) manualURL() string { return rs.srv.URL + "/downloads/g/f" }

func parallelGame(manualURL string) Game {
	return Game{
		Title: "Split Game",
		Downloads: []Downloadable{{
			Language:  "en",
			Platforms: Platform{Windows: []PlatformFile{{ManualURL: &manualURL, Name: "setup.exe"}}},
		}},
	}
}

func parallelBody(n int) []byte {
	body := make([]byte, n)
	for i := range body {
		body[i] = byte(i * 31)
	}
	return body
}

func TestDownloadGameFiles_ParallelAssemblesTheFile(t *testing.T) {
	smallParallelSizes(t, 1, 16*1024)
	body := parallelBody(100 * 1024)
	rs := newRangeServer(t, body)

	tmp := t.TempDir()
	require.NoError(t, DownloadGameFiles(context.Background(), "token", parallelGame(rs.manualURL()), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Connections: 3, Threads: 1}, io.Discard))

	filePath := filepath.Join(tmp, "split-game", "windows", "setup.exe")
	saved, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.True(t, bytes.Equal(body, saved), "the regions must reassemble into the file")
	require.GreaterOrEqual(t, len(rs.served()), 4, "a 100 KB file in 16 KB chunks takes several ranges")

	_, statErr := os.Stat(filePath + ".part.parallel")
	require.True(t, os.IsNotExist(statErr), "the sidecar must not outlive success")

	manifest, err := os.ReadFile(filepath.Join(tmp, "split-game", "files.json"))
	require.NoError(t, err)
	var files []DownloadedFile
	require.NoError(t, json.Unmarshal(manifest, &files))
	require.Len(t, files, 1)
	sum := md5.Sum(body)
	require.Equal(t, hex.EncodeToString(sum[:]), files[0].MD5,
		"the read-back checksum matches the file, not the download order")
}

func TestDownloadGameFiles_ParallelFallsBackWhenRangesAreIgnored(t *testing.T) {
	smallParallelSizes(t, 1, 16*1024)
	body := parallelBody(64 * 1024)
	rs := newRangeServer(t, body)
	rs.ignoreRanges = true

	tmp := t.TempDir()
	require.NoError(t, DownloadGameFiles(context.Background(), "token", parallelGame(rs.manualURL()), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Connections: 4, Threads: 1}, io.Discard))

	saved, err := os.ReadFile(filepath.Join(tmp, "split-game", "windows", "setup.exe"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(body, saved), "a server without ranges still gets one correct stream")
}

func TestDownloadGameFiles_SmallFileStaysOneStream(t *testing.T) {
	smallParallelSizes(t, 1<<20, 16*1024)
	body := parallelBody(8 * 1024)
	rs := newRangeServer(t, body)

	tmp := t.TempDir()
	require.NoError(t, DownloadGameFiles(context.Background(), "token", parallelGame(rs.manualURL()), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Connections: 8, Threads: 1}, io.Discard))

	saved, err := os.ReadFile(filepath.Join(tmp, "split-game", "windows", "setup.exe"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(body, saved))
	require.Empty(t, rs.served(), "a file under the threshold never asks for ranges")
}

// A parallel download cancelled midway leaves the sidecar behind, and the
// next run fetches only what is missing.
func TestDownloadGameFiles_ParallelResumesFromTheSidecar(t *testing.T) {
	smallParallelSizes(t, 1, 16*1024)
	body := parallelBody(128 * 1024)
	rs := newRangeServer(t, body)

	tmp := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the first run once a few chunks made it to disk.
	var once sync.Once
	inner := rs.srv.Config.Handler
	rs.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner.ServeHTTP(w, r)
		if len(rs.served()) >= 3 {
			once.Do(cancel)
		}
	})

	err := DownloadGameFiles(ctx, "token", parallelGame(rs.manualURL()), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Connections: 2, Resume: true, Threads: 1}, io.Discard)
	require.Error(t, err, "the cancelled first run must not report success")

	filePath := filepath.Join(tmp, "split-game", "windows", "setup.exe")
	require.FileExists(t, filePath+".part", "the partial file survives a cancelled resume-on run")
	require.FileExists(t, filePath+".part.parallel", "the sidecar survives with it")

	firstRun := len(rs.served())
	require.NoError(t, DownloadGameFiles(context.Background(), "token", parallelGame(rs.manualURL()), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Connections: 2, Resume: true, Threads: 1}, io.Discard))

	saved, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.True(t, bytes.Equal(body, saved), "both runs together produce the exact file")

	secondRun := len(rs.served()) - firstRun
	totalChunks := 128 / 16
	require.Less(t, secondRun, totalChunks, "the second run must skip chunks the first already landed")
}

// A holey .part file guarded by a sidecar must never be appended to by the
// sequential path: with one connection, the download starts over.
func TestDownloadGameFiles_SequentialStartsOverAfterAParallelInterrupt(t *testing.T) {
	smallParallelSizes(t, 1, 16*1024)
	body := parallelBody(64 * 1024)
	rs := newRangeServer(t, body)

	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "split-game", "windows")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	partPath := filepath.Join(targetDir, "setup.exe.part")

	// A holey partial: full size, but only the second chunk ever landed.
	holey := make([]byte, len(body))
	copy(holey[16*1024:32*1024], body[16*1024:32*1024])
	require.NoError(t, os.WriteFile(partPath, holey, 0o644))
	sidecar, err := json.Marshal(parallelState{TotalSize: int64(len(body)), ChunkSize: 16 * 1024, Done: []int{1}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(partPath+".parallel", sidecar, 0o644))

	require.NoError(t, DownloadGameFiles(context.Background(), "token", parallelGame(rs.manualURL()), tmp,
		DownloadOptions{Language: "en", Platform: "windows", Connections: 1, Resume: true, Threads: 1}, io.Discard))

	saved, err := os.ReadFile(filepath.Join(targetDir, "setup.exe"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(body, saved), "the sequential path must not resume into the holes")

	_, statErr := os.Stat(partPath + ".parallel")
	require.True(t, os.IsNotExist(statErr), "the stale sidecar is cleaned up")
}
