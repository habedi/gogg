package gui

import (
	"os"
	"testing"
)

// TestMain works the download statuses out where the test that asked for them
// can see them. On a thread of its own the work outlives the test that started
// it, and lands in the next one.
func TestMain(m *testing.M) {
	statusWorker = func(work func() gameStatuses, apply func(gameStatuses)) { apply(work()) }
	os.Exit(m.Run())
}
