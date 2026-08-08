package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubRetryWait replaces the retry wait for the duration of the test. It
// records the delays that were asked for and returns waitErr instead of
// sleeping, which keeps retry tests free of wall-clock assertions.
func stubRetryWait(t *testing.T, waitErr error) func() []time.Duration {
	t.Helper()
	var delays []time.Duration
	original := waitBeforeRetry
	waitBeforeRetry = func(_ context.Context, d time.Duration) error {
		delays = append(delays, d)
		return waitErr
	}
	t.Cleanup(func() { waitBeforeRetry = original })
	return func() []time.Duration { return delays }
}

func alwaysFailingServer(t *testing.T) (*httptest.Server, func() int64) {
	t.Helper()
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv, attempts.Load
}

// The delay doubles between attempts, and there is no wait after the final one
// because there is nothing left to wait for.
func TestSendRequest_WaitsBetweenAttemptsButNotAfterTheLast(t *testing.T) {
	srv, attempts := alwaysFailingServer(t)
	delays := stubRetryWait(t, nil)

	req, err := http.NewRequest("GET", srv.URL, nil)
	require.NoError(t, err)

	_, err = sendRequest(req)
	require.Error(t, err)

	require.Equal(t, int64(3), attempts(), "three attempts")
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, delays(),
		"two waits for three attempts, doubling each time")
}

// Cancelling the command (Ctrl-C, or --timeout) during a backoff stops the
// retries instead of running the schedule to completion.
func TestSendRequest_StopsWhenTheWaitIsCancelled(t *testing.T) {
	srv, attempts := alwaysFailingServer(t)
	stubRetryWait(t, context.Canceled)

	req, err := http.NewRequest("GET", srv.URL, nil)
	require.NoError(t, err)

	_, err = sendRequest(req)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int64(1), attempts(), "no further attempts after cancellation")
}

func TestWaitBeforeRetry_ReturnsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// A delay this long would hang the test if cancellation were ignored.
	err := waitBeforeRetry(ctx, time.Hour)
	require.ErrorIs(t, err, context.Canceled)
}

func TestWaitBeforeRetry_ReturnsWhenTheDelayElapses(t *testing.T) {
	require.NoError(t, waitBeforeRetry(context.Background(), time.Millisecond))
}

func TestWaitBeforeRetry_IsUsedBySendRequest(t *testing.T) {
	// Guards against the wait being inlined again: a stub that reports an
	// unmistakable error has to surface through sendRequest.
	sentinel := errors.New("stub wait")
	srv, _ := alwaysFailingServer(t)
	stubRetryWait(t, sentinel)

	req, err := http.NewRequest("GET", srv.URL, nil)
	require.NoError(t, err)

	_, err = sendRequest(req)
	require.ErrorIs(t, err, sentinel)
}
