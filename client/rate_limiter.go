package client

import (
	"io"
	"sync"
	"time"
)

type RateLimiter struct {
	mu     sync.Mutex
	rate   int64   // bytes per second
	tokens float64 // current available tokens
	last   time.Time
}

var (
	GlobalDownloadRateLimiter *RateLimiter
	rateLimiterMu             sync.RWMutex
)

func SetGlobalDownloadRateLimit(bytesPerSecond int64) {
	rateLimiterMu.Lock()
	defer rateLimiterMu.Unlock()

	if bytesPerSecond <= 0 {
		GlobalDownloadRateLimiter = nil
		return
	}
	if GlobalDownloadRateLimiter == nil {
		GlobalDownloadRateLimiter = &RateLimiter{rate: bytesPerSecond, tokens: float64(bytesPerSecond), last: time.Now()}
	} else {
		GlobalDownloadRateLimiter.mu.Lock()
		GlobalDownloadRateLimiter.rate = bytesPerSecond
		if GlobalDownloadRateLimiter.tokens > float64(bytesPerSecond) {
			GlobalDownloadRateLimiter.tokens = float64(bytesPerSecond)
		}
		GlobalDownloadRateLimiter.last = time.Now()
		GlobalDownloadRateLimiter.mu.Unlock()
	}
}

type limitedReader struct {
	under io.Reader
	lim   *RateLimiter
}

func (lr *limitedReader) Read(p []byte) (int, error) {
	if lr.lim == nil || lr.lim.rate <= 0 {
		return lr.under.Read(p)
	}
	lr.lim.mu.Lock()
	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(lr.lim.last).Seconds()
	if elapsed > 0 {
		lr.lim.tokens += elapsed * float64(lr.lim.rate)
		maxTokens := float64(lr.lim.rate)
		if lr.lim.tokens > maxTokens {
			lr.lim.tokens = maxTokens
		}
		lr.lim.last = now
	}
	// Decide max bytes we can read now
	allowed := int(lr.lim.tokens)
	if allowed <= 0 {
		// Need to wait for next refill cycle
		lr.lim.mu.Unlock()
		sleepDur := time.Duration(float64(time.Second) * (1.0 / float64(lr.lim.rate)))
		time.Sleep(sleepDur)
		return lr.Read(p)
	}
	if len(p) > allowed {
		p = p[:allowed]
	}
	lr.lim.mu.Unlock()
	n, err := lr.under.Read(p)
	if n > 0 {
		lr.lim.mu.Lock()
		lr.lim.tokens -= float64(n)
		lr.lim.mu.Unlock()
	}
	return n, err
}

func wrapWithGlobalRateLimiter(r io.Reader) io.Reader {
	rateLimiterMu.RLock()
	lim := GlobalDownloadRateLimiter
	rateLimiterMu.RUnlock()

	if lim == nil {
		return r
	}
	return &limitedReader{under: r, lim: lim}
}
