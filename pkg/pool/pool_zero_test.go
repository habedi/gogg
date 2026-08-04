package pool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// A caller that passes a non-positive worker count must still make progress
// rather than blocking forever on a channel nobody reads.
func TestRun_NonPositiveWorkerCount(t *testing.T) {
	for _, workers := range []int{0, -1} {
		var processed atomic.Int64
		done := make(chan []error, 1)

		go func() {
			done <- Run(context.Background(), []int{1, 2, 3}, workers, func(_ context.Context, _ int) error {
				processed.Add(1)
				return nil
			})
		}()

		select {
		case errs := <-done:
			if len(errs) != 0 {
				t.Errorf("workers=%d: unexpected errors: %v", workers, errs)
			}
			if got := processed.Load(); got != 3 {
				t.Errorf("workers=%d: processed %d items, want 3", workers, got)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("workers=%d: Run deadlocked", workers)
		}
	}
}
