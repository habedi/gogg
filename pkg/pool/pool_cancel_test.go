package pool_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/habedi/gogg/pkg/pool"
)

func TestPool_CancelStopsEnqueue(t *testing.T) {
	items := make([]int, 1000)
	for i := range items {
		items[i] = i
	}
	ctx, cancel := context.WithCancel(context.Background())
	var processed int64

	worker := func(ctx context.Context, i int) error {
		atomic.AddInt64(&processed, 1)
		if i == 0 {
			cancel()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Millisecond):
		}
		return nil
	}

	_ = pool.Run(ctx, items, 8, worker)
	if atomic.LoadInt64(&processed) >= int64(len(items)) {
		t.Fatalf("expected fewer items processed after cancel, got %d", processed)
	}
}
