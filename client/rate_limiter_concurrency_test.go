package client

import (
	"strings"
	"testing"
)

// Changing the download speed limit while a download is reading through the
// limiter must not race. Run with -race.
func TestLimitedReader_ConcurrentRateChange(t *testing.T) {
	SetGlobalDownloadRateLimit(1 << 20)
	t.Cleanup(func() { SetGlobalDownloadRateLimit(0) })

	reader := wrapWithGlobalRateLimiter(strings.NewReader(strings.Repeat("a", 1<<16)))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			SetGlobalDownloadRateLimit(int64(1<<20) + int64(i))
		}
	}()

	buf := make([]byte, 512)
	for {
		if _, err := reader.Read(buf); err != nil {
			break
		}
	}
	<-done
}

// Switching the limiter off mid-download must not race either.
func TestSetGlobalDownloadRateLimit_DisableWhileReading(t *testing.T) {
	SetGlobalDownloadRateLimit(1 << 20)
	t.Cleanup(func() { SetGlobalDownloadRateLimit(0) })

	reader := wrapWithGlobalRateLimiter(strings.NewReader(strings.Repeat("a", 1<<16)))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			SetGlobalDownloadRateLimit(0)
			SetGlobalDownloadRateLimit(1 << 20)
		}
	}()

	buf := make([]byte, 512)
	for {
		if _, err := reader.Read(buf); err != nil {
			break
		}
	}
	<-done
}
