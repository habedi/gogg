package pool

import (
	"context"
	"sync"
)

// WorkerFunc defines the function signature for a worker that processes an item and may return an error.
type WorkerFunc[T any] func(ctx context.Context, item T) error

// Run executes a worker pool. It processes a slice of items concurrently.
// It returns a slice containing any errors that occurred during processing.
func Run[T any](ctx context.Context, items []T, numWorkers int, workerFunc WorkerFunc[T]) []error {
	var wg sync.WaitGroup
	taskChan := make(chan T, numWorkers)
	errChan := make(chan error, len(items))

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range taskChan {
				select {
				case <-ctx.Done():
					return
				default:
					if err := workerFunc(ctx, item); err != nil {
						errChan <- err
					}
				}
			}
		}()
	}

OUT:
	for _, item := range items {
		select {
		case taskChan <- item:
		case <-ctx.Done():
			// Stop feeding tasks if the context is cancelled
			break OUT
		}
	}
	close(taskChan)

	wg.Wait()
	close(errChan)

	var allErrors []error
	for err := range errChan {
		allErrors = append(allErrors, err)
	}
	return allErrors
}
