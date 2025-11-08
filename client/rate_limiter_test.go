package client

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"
)

func TestSetGlobalDownloadRateLimit_ZeroAndNegative(t *testing.T) {
	tests := []struct {
		name  string
		limit int64
	}{
		{"zero limit", 0},
		{"negative limit", -100},
		{"very negative", -9999999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetGlobalDownloadRateLimit(tt.limit)

			rateLimiterMu.RLock()
			isNil := GlobalDownloadRateLimiter == nil
			rateLimiterMu.RUnlock()

			if !isNil {
				t.Errorf("SetGlobalDownloadRateLimit(%d) should set limiter to nil", tt.limit)
			}
		})
	}
}

func TestSetGlobalDownloadRateLimit_Positive(t *testing.T) {
	tests := []struct {
		name  string
		limit int64
	}{
		{"small limit", 100},
		{"medium limit", 1024 * 1024},
		{"large limit", 100 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetGlobalDownloadRateLimit(tt.limit)

			rateLimiterMu.RLock()
			limiter := GlobalDownloadRateLimiter
			rateLimiterMu.RUnlock()

			if limiter == nil {
				t.Errorf("SetGlobalDownloadRateLimit(%d) should create limiter", tt.limit)
				return
			}

			if limiter.rate != tt.limit {
				t.Errorf("rate = %d, want %d", limiter.rate, tt.limit)
			}
		})
	}
}

func TestSetGlobalDownloadRateLimit_Update(t *testing.T) {
	// Set initial limit
	SetGlobalDownloadRateLimit(1000)

	// Update to new limit
	SetGlobalDownloadRateLimit(2000)

	rateLimiterMu.RLock()
	limiter := GlobalDownloadRateLimiter
	rateLimiterMu.RUnlock()

	if limiter == nil {
		t.Fatal("Limiter should not be nil after update")
	}

	if limiter.rate != 2000 {
		t.Errorf("Updated rate = %d, want 2000", limiter.rate)
	}
}

func TestSetGlobalDownloadRateLimit_TokensCapped(t *testing.T) {
	// Set high limit with lots of tokens
	SetGlobalDownloadRateLimit(1000)

	rateLimiterMu.RLock()
	limiter := GlobalDownloadRateLimiter
	rateLimiterMu.RUnlock()

	if limiter == nil {
		t.Fatal("Limiter should not be nil")
	}

	// Manually set tokens very high
	limiter.mu.Lock()
	limiter.tokens = 100000
	limiter.mu.Unlock()

	// Update to lower limit
	SetGlobalDownloadRateLimit(500)

	// Tokens should be capped to new rate
	limiter.mu.Lock()
	tokens := limiter.tokens
	limiter.mu.Unlock()

	if tokens > 500 {
		t.Errorf("Tokens = %f, should be capped at 500", tokens)
	}
}

func TestSetGlobalDownloadRateLimit_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(val int64) {
			defer wg.Done()
			SetGlobalDownloadRateLimit(val)
		}(int64(i * 100))
	}
	wg.Wait()

	// Should not panic and should have some valid limiter
	rateLimiterMu.RLock()
	limiter := GlobalDownloadRateLimiter
	rateLimiterMu.RUnlock()

	if limiter == nil {
		t.Error("Concurrent calls resulted in nil limiter")
	}
}

func TestWrapWithGlobalRateLimiter_Nil(t *testing.T) {
	SetGlobalDownloadRateLimit(0) // Set to nil

	reader := bytes.NewReader([]byte("test data"))
	wrapped := wrapWithGlobalRateLimiter(reader)

	// Should be able to read without rate limiting regardless of wrapper type
	buf := make([]byte, 100)
	n, err := wrapped.Read(buf)
	if err != nil && err != io.EOF {
		t.Errorf("Read failed: %v", err)
	}
	if n == 0 {
		t.Error("Expected to read some bytes")
	}
}

func TestWrapWithGlobalRateLimiter_WithLimit(t *testing.T) {
	SetGlobalDownloadRateLimit(1024)

	reader := bytes.NewReader([]byte("test data"))
	wrapped := wrapWithGlobalRateLimiter(reader)

	// Should be able to read
	buf := make([]byte, 100)
	n, err := wrapped.Read(buf)
	if err != nil && err != io.EOF {
		t.Errorf("Read failed: %v", err)
	}
	if n == 0 {
		t.Error("Expected to read some bytes")
	}
}

func TestLimitedReader_NoLimit(t *testing.T) {
	data := []byte("test data for reading")
	reader := bytes.NewReader(data)
	lr := &limitedReader{under: reader, lim: nil}

	buf := make([]byte, len(data))
	n, err := lr.Read(buf)

	if err != nil && err != io.EOF {
		t.Errorf("Read failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Read %d bytes, want %d", n, len(data))
	}
	if !bytes.Equal(buf[:n], data) {
		t.Error("Data mismatch")
	}
}

func TestLimitedReader_ZeroRate(t *testing.T) {
	data := []byte("test")
	reader := bytes.NewReader(data)
	limiter := &RateLimiter{rate: 0, tokens: 0, last: time.Now()}
	lr := &limitedReader{under: reader, lim: limiter}

	// Should pass through without limiting
	buf := make([]byte, len(data))
	n, err := lr.Read(buf)

	if err != nil && err != io.EOF {
		t.Errorf("Read failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Read %d bytes, want %d", n, len(data))
	}
}

func TestLimitedReader_SmallBuffer(t *testing.T) {
	data := []byte("test data")
	reader := bytes.NewReader(data)
	limiter := &RateLimiter{rate: 1024, tokens: 1024, last: time.Now()}
	lr := &limitedReader{under: reader, lim: limiter}

	// Read with very small buffer
	buf := make([]byte, 2)
	n, err := lr.Read(buf)

	if err != nil && err != io.EOF {
		t.Errorf("Read failed: %v", err)
	}
	if n > 2 {
		t.Errorf("Read %d bytes, should not exceed buffer size 2", n)
	}
}

func TestLimitedReader_TokenRefill(t *testing.T) {
	data := make([]byte, 1000)
	reader := bytes.NewReader(data)

	// Start with no tokens, but a high rate
	limiter := &RateLimiter{
		rate:   10000,
		tokens: 0,
		last:   time.Now().Add(-2 * time.Second), // 2 seconds ago
	}
	lr := &limitedReader{under: reader, lim: limiter}

	// Should refill tokens based on elapsed time
	done := make(chan struct{})
	var n int
	var err error
	go func() {
		buf := make([]byte, 100)
		n, err = lr.Read(buf)
		close(done)
	}()

	select {
	case <-done:
		if err != nil && err != io.EOF {
			t.Errorf("Read failed: %v", err)
		}
		if n == 0 {
			t.Error("Should have read some bytes after token refill")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible infinite loop in rate limiter")
	}
}

func TestLimitedReader_LargeBufferCapped(t *testing.T) {
	data := make([]byte, 10000)
	reader := bytes.NewReader(data)

	// Low rate limits available bytes
	limiter := &RateLimiter{rate: 100, tokens: 50, last: time.Now()}
	lr := &limitedReader{under: reader, lim: limiter}

	// Try to read more than available tokens
	buf := make([]byte, 1000)
	n, err := lr.Read(buf)

	if err != nil && err != io.EOF {
		t.Errorf("Read failed: %v", err)
	}
	// Should be capped by available tokens
	if n > 100 {
		t.Errorf("Read %d bytes, should be capped by rate limit", n)
	}
}

func TestLimitedReader_TokensDeducted(t *testing.T) {
	data := []byte("test data for reading")
	reader := bytes.NewReader(data)

	limiter := &RateLimiter{rate: 1000, tokens: 1000, last: time.Now()}
	lr := &limitedReader{under: reader, lim: limiter}

	initialTokens := limiter.tokens

	buf := make([]byte, 10)
	n, _ := lr.Read(buf)

	limiter.mu.Lock()
	finalTokens := limiter.tokens
	limiter.mu.Unlock()

	expectedDeduction := float64(n)
	actualDeduction := initialTokens - finalTokens

	if actualDeduction != expectedDeduction {
		t.Errorf("Token deduction = %f, want %f", actualDeduction, expectedDeduction)
	}
}

func TestLimitedReader_ConcurrentReads(t *testing.T) {
	data := make([]byte, 10000)
	for i := range data {
		data[i] = byte(i % 256)
	}

	limiter := &RateLimiter{rate: 1000000, tokens: 1000000, last: time.Now()}

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader := bytes.NewReader(data)
			lr := &limitedReader{under: reader, lim: limiter}

			buf := make([]byte, 100)
			_, err := lr.Read(buf)
			if err != nil && err != io.EOF {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent read error: %v", err)
	}
}

func TestRateLimiter_TokenCapRespected(t *testing.T) {
	limiter := &RateLimiter{
		rate:   1000,
		tokens: 500,
		last:   time.Now().Add(-10 * time.Second), // Long time ago
	}

	// Refill should happen but be capped at rate
	data := []byte("test")
	reader := bytes.NewReader(data)
	lr := &limitedReader{under: reader, lim: limiter}

	buf := make([]byte, 10)
	lr.Read(buf)

	limiter.mu.Lock()
	tokens := limiter.tokens
	limiter.mu.Unlock()

	// Tokens should not exceed rate (1000)
	if tokens > float64(limiter.rate) {
		t.Errorf("Tokens %f exceed rate %d", tokens, limiter.rate)
	}
}
