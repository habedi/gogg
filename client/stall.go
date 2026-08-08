package client

import (
	"fmt"
	"io"
	"time"
)

// StallTimeout is how long a transfer may go without a single byte before it
// is cut off and handed to the retry machinery. A dead connection otherwise
// hangs a download until TCP gives up, which can be most of an hour. A
// variable so tests do not sit the window out.
var StallTimeout = 90 * time.Second

// stallError says a transfer was cut off for silence. It is retryable, and
// with resume on, the next attempt picks up at the byte where the line went
// quiet.
type stallError struct {
	window time.Duration
}

func (e *stallError) Error() string {
	return fmt.Sprintf("download stalled: no data received for %s", e.window)
}

// stallGuard postpones a watchdog every time bytes actually arrive. The
// watchdog itself is armed by the caller, to cancel the transfer's request;
// a Read blocked on a dead connection cannot notice its own silence.
type stallGuard struct {
	reader io.Reader
	timer  *time.Timer
	window time.Duration
}

func (s *stallGuard) Read(p []byte) (int, error) {
	n, err := s.reader.Read(p)
	if n > 0 {
		s.timer.Reset(s.window)
	}
	return n, err
}
