//go:build integration

package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type progressCollector struct {
	updates []ProgressUpdate
}

func (pc *progressCollector) Write(p []byte) (int, error) {
	s := bufio.NewScanner(strings.NewReader(string(p)))
	for s.Scan() {
		var u ProgressUpdate
		if err := json.Unmarshal(s.Bytes(), &u); err == nil {
			pc.updates = append(pc.updates, u)
		}
	}
	return len(p), nil
}

func makeTestGame(manual string) Game {
	mu := manual
	return Game{
		Title: "Test Game",
		Downloads: []Downloadable{
			{
				Language: "English",
				Platforms: Platform{
					Windows: []PlatformFile{{ManualURL: &mu, Name: "setup.exe", Size: "11"}},
				},
			},
		},
	}
}

func TestDownloadGameFiles_SimpleAndResume(t *testing.T) {
	content := []byte("hello world")

	var supportRange bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redir_to":
			to := r.URL.Query().Get("u")
			w.Header().Set("Location", to)
			w.WriteHeader(http.StatusFound)
			return
		case "/file":
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
			if r.Method == http.MethodHead {
				return
			}
			if supportRange {
				rg := r.Header.Get("Range")
				if strings.HasPrefix(rg, "bytes=") {
					startStr := strings.TrimPrefix(rg, "bytes=")
					startStr = strings.TrimSuffix(startStr, "-")
					var start int
					fmt.Sscanf(startStr, "%d", &start)
					if start > 0 {
						w.WriteHeader(http.StatusPartialContent)
						w.Write(content[start:])
						return
					}
				}
			}
			w.WriteHeader(http.StatusOK)
			w.Write(content)
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	rel := "/redir_to?u=" + url.QueryEscape(server.URL+"/file")
	game := makeTestGame(rel)
	dir := t.TempDir()
	pc := &progressCollector{}

	// Simple download without resume
	supportRange = true
	err := DownloadGameFiles(context.Background(), "token", game, dir, "English", "windows", false, false, false, true, false, 2, pc)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, SanitizePath(game.Title), "windows", "setup.exe"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("content mismatch: got %q want %q", string(data), string(content))
	}

	// Resume download: create partial file and ensure resume appends remaining
	pc2 := &progressCollector{}
	filePath := filepath.Join(dir, SanitizePath(game.Title), "windows", "setup.exe")
	if err := os.WriteFile(filePath, content[:5], 0644); err != nil {
		t.Fatalf("prepare partial: %v", err)
	}
	err = DownloadGameFiles(context.Background(), "token", game, dir, "English", "windows", false, false, true, true, false, 2, pc2)
	if err != nil {
		t.Fatalf("resume download failed: %v", err)
	}
	data, err = os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read after resume: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("resume content mismatch: got %q want %q", string(data), string(content))
	}

	// Range ignored: server returns 200 always; expect truncation and redownload
	supportRange = false
	if err := os.WriteFile(filePath, content[:5], 0644); err != nil {
		t.Fatalf("prepare partial for ignore-range: %v", err)
	}
	pc3 := &progressCollector{}
	err = DownloadGameFiles(context.Background(), "token", game, dir, "English", "windows", false, false, true, true, false, 1, pc3)
	if err != nil {
		t.Fatalf("ignore-range resume download failed: %v", err)
	}
	data, err = os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read after ignore-range: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("ignore-range content mismatch: got %q want %q", string(data), string(content))
	}
}
