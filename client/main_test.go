package client

import (
	"os"
	"testing"
	"time"
)

// TestMain shrinks the retry pause: the tests that make a download fail on
// purpose should not sit out the waits a real dropped connection deserves.
func TestMain(m *testing.M) {
	RetryDelay = time.Millisecond
	os.Exit(m.Run())
}
