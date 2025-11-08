package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func buildTestBinary(t *testing.T) string {
	binName := "gogg_it_bin"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(t.TempDir(), binName)
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, string(out))
	}
	return bin
}

// TestTimeoutContext make sure a short timeout cancels a long-running catalogue refresh (simulated by sleep via env flag).
func TestTimeoutContext(t *testing.T) {
	bin := buildTestBinary(t)
	start := time.Now()
	cmd := exec.Command(bin, "catalogue", "list", "-T", "500ms")
	cmd.Env = os.Environ()
	err := cmd.Run()
	elapsed := time.Since(start)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("unexpected error type: %v", err)
		} else if elapsed > time.Second*2 {
			t.Fatalf("timeout test exceeded expected duration: %v", elapsed)
		}
	}
	if elapsed > time.Second*2 {
		t.Fatalf("list command took too long with timeout flag: %v", elapsed)
	}
}

// TestGracefulInterrupt runs the binary and sends SIGINT, expecting it to exit promptly.
func TestGracefulInterrupt(t *testing.T) {
	bin := buildTestBinary(t)
	cmd := exec.Command(bin, "catalogue", "list")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}
	// Allow startup
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("failed to send interrupt: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		// Accept any exit code; main uses exit code 1 on interrupt.
		_ = err
	case <-time.After(3 * time.Second):
		t.Fatal("process did not exit within 3s after SIGINT")
	}
}
