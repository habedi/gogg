package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Cancelling the command (Ctrl-C, or --timeout) must interrupt the retry
// backoff instead of running the whole retry schedule to completion.
func TestSendRequest_HonorsCancellationDuringBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	if _, err := sendRequest(req); err == nil {
		t.Fatal("expected an error")
	}
	if elapsed := time.Since(start); elapsed > 900*time.Millisecond {
		t.Errorf("sendRequest ignored cancellation: took %s", elapsed)
	}
}

// After the final attempt there is nothing left to wait for, so the last
// backoff must not be slept through.
func TestSendRequest_NoBackoffAfterFinalAttempt(t *testing.T) {
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	req, err := http.NewRequest("GET", srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if _, err := sendRequest(req); err == nil {
		t.Fatal("expected an error")
	}
	elapsed := time.Since(start)

	if got := attempts.Load(); got != 3 {
		t.Errorf("got %d attempts, want 3", got)
	}
	// Backoffs are 1s and 2s between the three attempts; a 4s sleep after the
	// last one is pure waste.
	if elapsed > 4*time.Second {
		t.Errorf("slept after the final attempt: took %s", elapsed)
	}
}
