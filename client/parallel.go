package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

// Adapted from the parallel downloader Lutris ships for GOG's CDN: a large
// file is split into fixed regions, and a few connections each fill their own
// region with HTTP range requests. The CDN serves ranges for resumption
// already, so nothing new is asked of the server.

var (
	// parallelMinSize is the smallest file worth splitting across
	// connections; anything smaller downloads in one stream. A variable so
	// tests do not need gigabyte fixtures.
	parallelMinSize int64 = 32 << 20
	// parallelChunkSize is the region one range request claims.
	parallelChunkSize int64 = 8 << 20
)

// errServerIgnoredRange says a ranged request came back as a full 200, so
// the file cannot be split and must be fetched in one stream instead.
var errServerIgnoredRange = errors.New("server ignored the range request")

// parallelState is the sidecar written beside a .part file while connections
// fill it, recording which chunks made it to disk. It is what allows an
// interrupted parallel download to resume: the .part file alone has holes,
// and the sequential resume logic must never append after a hole.
type parallelState struct {
	TotalSize int64 `json:"total_size"`
	ChunkSize int64 `json:"chunk_size"`
	Done      []int `json:"done"`
}

func parallelSidecarPath(partPath string) string { return partPath + ".parallel" }

// loadParallelState reads a sidecar and vouches for it. Anything unreadable,
// or written for a file of a different size, counts as absent: the download
// then starts over rather than trusting stale bookkeeping.
func loadParallelState(path string, totalSize int64) *parallelState {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var state parallelState
	if json.Unmarshal(data, &state) != nil || state.ChunkSize <= 0 || state.TotalSize != totalSize {
		return nil
	}
	return &state
}

// downloadFileParallel fills partPath with totalSize bytes using several
// range connections, reporting progress the same way the single stream does.
// On success the .part file is complete and the sidecar is gone; on failure
// the sidecar records what is already on disk, ready for the next attempt.
func downloadFileParallel(
	ctx context.Context,
	httpClient *http.Client,
	accessToken, url, partPath, fileName string,
	totalSize int64,
	connections int,
	sw io.Writer,
) error {
	sidecarPath := parallelSidecarPath(partPath)
	state := loadParallelState(sidecarPath, totalSize)
	chunkSize := parallelChunkSize
	if state != nil {
		chunkSize = state.ChunkSize
	}
	numChunks := int((totalSize + chunkSize - 1) / chunkSize)

	chunkLen := func(idx int) int64 {
		start := int64(idx) * chunkSize
		end := start + chunkSize
		if end > totalSize {
			end = totalSize
		}
		return end - start
	}

	file, err := os.OpenFile(partPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if err := file.Truncate(totalSize); err != nil {
		return err
	}

	done := make(map[int]bool, numChunks)
	if state != nil {
		for _, idx := range state.Done {
			if idx >= 0 && idx < numChunks {
				done[idx] = true
			}
		}
	}
	var doneBytes int64
	var pending []int
	for idx := 0; idx < numChunks; idx++ {
		if done[idx] {
			doneBytes += chunkLen(idx)
		} else {
			pending = append(pending, idx)
		}
	}

	progress := &progressReader{
		writer:    sw,
		fileName:  fileName,
		totalSize: totalSize,
		bytesRead: doneBytes,
	}

	var mu sync.Mutex
	persistLocked := func() {
		indices := make([]int, 0, len(done))
		for idx := range done {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		data, err := json.Marshal(parallelState{TotalSize: totalSize, ChunkSize: chunkSize, Done: indices})
		if err != nil {
			return
		}
		_ = os.WriteFile(sidecarPath, data, 0644)
	}

	groupCtx, cancelGroup := context.WithCancel(ctx)
	defer cancelGroup()
	var failOnce sync.Once
	var firstErr error
	fail := func(err error) {
		failOnce.Do(func() {
			firstErr = err
			cancelGroup()
		})
	}

	jobs := make(chan int)
	go func() {
		defer close(jobs)
		for _, idx := range pending {
			select {
			case jobs <- idx:
			case <-groupCtx.Done():
				return
			}
		}
	}()

	workers := connections
	if workers > len(pending) {
		workers = len(pending)
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				start := int64(idx) * chunkSize
				if err := downloadChunkWithRetry(groupCtx, httpClient, accessToken, url,
					file, start, chunkLen(idx), progress); err != nil {
					fail(err)
					return
				}
				mu.Lock()
				done[idx] = true
				persistLocked()
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	progress.report(true)
	_ = os.Remove(sidecarPath)
	return nil
}

// downloadChunkWithRetry fetches one region, with the same attempt count and
// backoff a whole file gets in the single stream. Progress counted for a
// failed attempt is taken back, so a retried chunk is not reported twice.
func downloadChunkWithRetry(
	ctx context.Context,
	httpClient *http.Client,
	accessToken, url string,
	file *os.File,
	start, length int64,
	progress *progressReader,
) error {
	var err error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(RetryDelay << (attempt - 2)):
			}
		}
		err = downloadChunk(ctx, httpClient, accessToken, url, file, start, length, progress)
		if err == nil || isCancellation(err) || errors.Is(err, errServerIgnoredRange) || !isRetryable(err) {
			return err
		}
	}
	return err
}

func downloadChunk(
	ctx context.Context,
	httpClient *http.Client,
	accessToken, url string,
	file *os.File,
	start, length int64,
	progress *progressReader,
) (err error) {
	stallCtx, cancelTransfer := context.WithCancelCause(ctx)
	defer cancelTransfer(nil)

	req, err := http.NewRequestWithContext(stallCtx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, start+length-1))

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		return errServerIgnoredRange
	case http.StatusForbidden:
		// The HEAD for this file already passed, so a 403 partway through
		// means the signed link went stale rather than a refusal. The error
		// stays untyped on purpose: it must be retried, and the retry
		// resolves a fresh link.
		return fmt.Errorf("range %d-%d refused with HTTP 403; the download link likely expired", start, start+length-1)
	default:
		return fmt.Errorf("range %d-%d failed: %w", start, start+length-1, &httpStatusError{status: resp.StatusCode})
	}

	stallWindow := StallTimeout
	watchdog := time.AfterFunc(stallWindow, func() {
		cancelTransfer(&stallError{window: stallWindow})
	})
	defer watchdog.Stop()

	var counted int64
	defer func() {
		if err != nil {
			progress.add(-counted)
		}
	}()

	reader := &stallGuard{
		reader: wrapWithGlobalRateLimiter(io.LimitReader(resp.Body, length)),
		timer:  watchdog,
		window: stallWindow,
	}
	buffer := make([]byte, 32*1024)
	written, err := io.CopyBuffer(io.NewOffsetWriter(file, start), &countingReader{reader: reader, counted: &counted, progress: progress}, buffer)
	if err != nil {
		var stalled *stallError
		if errors.As(context.Cause(stallCtx), &stalled) {
			return stalled
		}
		return err
	}
	if written != length {
		return fmt.Errorf("range %d-%d returned %d of %d bytes", start, start+length-1, written, length)
	}
	return nil
}

// countingReader feeds the shared progress reporter and remembers how much,
// so a failed chunk can give its count back.
type countingReader struct {
	reader   io.Reader
	counted  *int64
	progress *progressReader
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if n > 0 {
		*c.counted += int64(n)
		c.progress.add(int64(n))
	}
	return n, err
}
