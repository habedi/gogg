package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/habedi/gogg/auth"
)

func TestDownloadCmd_InvalidID(t *testing.T) {
	authService := auth.NewService(nil, nil)
	cmd := downloadCmd(authService)
	dir := t.TempDir()
	cmd.SetArgs([]string{"abc", dir})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	// Should not panic or exit; prints error
	cmd.Execute()
	if got := buf.String(); got == "" {
		t.Fatalf("expected error output, got empty")
	}
}

// captureStdout runs f while capturing os.Stdout and returns captured output.
func captureStdout2(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	_ = r.Close()
	return string(out)
}

func TestExecuteDownload_InvalidLanguagePrintsList(t *testing.T) {
	// Use invalid language code to trigger early return and listing of supported languages
	out := captureStdout2(func() {
		executeDownload(context.Background(), nil, 1, filepath.Join(t.TempDir(), "dl"), "xx", "windows", true, true, true, true, false, 2)
	})
	if out == "" {
		t.Fatalf("expected output for invalid language")
	}
	if !containsAll(out, []string{"Invalid language code", "'en'"}) {
		t.Fatalf("unexpected output: %s", out)
	}
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !bytes.Contains([]byte(s), []byte(sub)) {
			return false
		}
	}
	return true
}
