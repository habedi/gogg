package pool_test

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/habedi/gogg/pkg/pool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPool_Run(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	var count atomic.Int64

	workerFunc := func(ctx context.Context, item int) error {
		count.Add(1)
		time.Sleep(10 * time.Millisecond) // Simulate work
		return nil
	}

	errors := pool.Run(context.Background(), items, 3, workerFunc)

	assert.Empty(t, errors)
	assert.Equal(t, int64(len(items)), count.Load())
}

func TestPool_CollectsErrors(t *testing.T) {
	items := []int{1, 2, 3, 4}
	expectedErr := errors.New("worker failed")

	workerFunc := func(ctx context.Context, item int) error {
		if item%2 == 0 {
			return expectedErr
		}
		return nil
	}

	errs := pool.Run(context.Background(), items, 2, workerFunc)
	require.Len(t, errs, 2)
	assert.ErrorIs(t, errs[0], expectedErr)
	assert.ErrorIs(t, errs[1], expectedErr)
}

func TestPool_ContextCancellation(t *testing.T) {
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}
	var processedCount atomic.Int64

	ctx, cancel := context.WithCancel(context.Background())

	workerFunc := func(ctx context.Context, item int) error {
		processedCount.Add(1)
		// Cancel the context after the first item is processed
		if item == 0 {
			cancel()
		}
		// A realistic worker would check the context
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
		return nil
	}

	pool.Run(ctx, items, runtime.NumCPU(), workerFunc)

	// Due to the nature of concurrency, we can't assert an exact number.
	// But it should be much less than the total number of items.
	assert.Less(t, processedCount.Load(), int64(len(items)), "Pool should stop processing after context is cancelled")
}
