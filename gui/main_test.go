package gui

import (
	"os"
	"testing"
	"time"

	"github.com/habedi/gogg/client"
)

// TestMain works the download statuses out where the test that asked for them
// can see them. On a thread of its own the work outlives the test that started
// it, and lands in the next one.
func TestMain(m *testing.M) {
	statusWorker = func(work func() gameStatuses, apply func(gameStatuses)) { apply(work()) }
	// Searches filter as they are typed. A test types once and looks at once,
	// so waiting out the debounce would only make every test slower.
	searchDebounce = 0
	// Downloads that fail on purpose must not sit out the real retry pauses.
	client.RetryDelay = time.Millisecond
	os.Exit(m.Run())
}
