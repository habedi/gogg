package gui

import (
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// openFolder runs from a tap handler on the UI thread, so it must not wait for
// the file manager to be closed again.
func TestOpenFolder_DoesNotWaitForTheFileManager(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX sleep command")
	}

	original := folderCommand
	folderCommand = func(_ string) *exec.Cmd { return exec.Command("sleep", "5") }
	t.Cleanup(func() { folderCommand = original })

	done := make(chan struct{})
	go func() {
		defer close(done)
		openFolder(t.TempDir())
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("openFolder blocked until the file manager exited")
	}
}
