package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPartialCleanupOnCancel make sure partial files are removed when not resuming on ctx cancel.
func TestPartialCleanupOnCancel(t *testing.T) {
	// Fake server that serves a large content slowly
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1048576") // 1MB
		// Send bytes slowly
		buf := make([]byte, 1024)
		for i := 0; i < 1024; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			if _, err := w.Write(buf); err != nil {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}))
	defer srv.Close()

	tmp := t.TempDir()
	g := Game{Title: "Test Game", Downloads: []Downloadable{{Language: "English", Platforms: Platform{Windows: []PlatformFile{{Name: "file.bin", Size: "1 MB", ManualURL: strPtr(srv.URL)}}}}}}
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after starting
	go func() { time.Sleep(10 * time.Millisecond); cancel() }()
	err := DownloadGameFiles(ctx, "tok", g, tmp, "English", "windows", false, false, true, true, false, 1, os.Stdout)
	if err == nil {
		// Should cancel
		t.Fatal("expected cancellation error")
	}
	// Partial file should not exist due to resume=false & cleanup logic
	p := filepath.Join(tmp, SanitizePath(g.Title), "windows", "file.bin")
	if _, statErr := os.Stat(p); statErr == nil {
		t.Fatalf("expected partial file to be removed, found: %s", p)
	}
}

func strPtr(s string) *string { return &s }
