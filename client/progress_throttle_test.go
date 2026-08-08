package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// readAll drains src through a progressReader and returns the updates it wrote.
func readAll(t *testing.T, size, bufferSize int) []ProgressUpdate {
	t.Helper()

	out := new(bytes.Buffer)
	pr := &progressReader{
		reader:    bytes.NewReader(make([]byte, size)),
		writer:    out,
		fileName:  "file.bin",
		totalSize: int64(size),
	}

	buf := make([]byte, bufferSize)
	for {
		_, err := pr.Read(buf)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}

	var updates []ProgressUpdate
	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	for scanner.Scan() {
		var update ProgressUpdate
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &update))
		updates = append(updates, update)
	}
	return updates
}

// A 50 GB game read in 32 KiB chunks would otherwise emit over a million
// updates, each marshalled, serialised through a mutex and decoded again.
func TestProgressReader_ThrottlesUpdates(t *testing.T) {
	original := progressInterval
	progressInterval = time.Hour // nothing but the first and last may get through
	t.Cleanup(func() { progressInterval = original })

	const size, bufferSize = 1 << 20, 32 << 10 // 32 reads
	updates := readAll(t, size, bufferSize)

	require.Len(t, updates, 2, "one update when the transfer starts, one when it ends")
	require.Equal(t, int64(size), updates[len(updates)-1].CurrentBytes,
		"the final update has to report the whole file")
}

// Throttling must not lose the end of the transfer: the UI would be left
// showing a file as unfinished forever.
func TestProgressReader_AlwaysReportsTheFinalState(t *testing.T) {
	original := progressInterval
	progressInterval = time.Hour
	t.Cleanup(func() { progressInterval = original })

	updates := readAll(t, 4096, 1024)
	require.NotEmpty(t, updates)

	final := updates[len(updates)-1]
	require.Equal(t, int64(4096), final.CurrentBytes)
	require.Equal(t, int64(4096), final.TotalBytes)
	require.Equal(t, "file_progress", final.Type)
}

func TestProgressReader_ReportsEveryReadWhenNotThrottled(t *testing.T) {
	original := progressInterval
	progressInterval = 0
	t.Cleanup(func() { progressInterval = original })

	updates := readAll(t, 4096, 1024) // four reads
	require.GreaterOrEqual(t, len(updates), 4)
}

// A resumed download starts part way through, and the first update has to say
// so rather than restarting the count.
func TestProgressReader_CountsFromWhereItResumed(t *testing.T) {
	original := progressInterval
	progressInterval = time.Hour
	t.Cleanup(func() { progressInterval = original })

	out := new(bytes.Buffer)
	pr := &progressReader{
		reader:    bytes.NewReader(make([]byte, 1024)),
		writer:    out,
		fileName:  "file.bin",
		totalSize: 5120,
		bytesRead: 4096, // already on disk
	}

	buf := make([]byte, 512)
	for {
		if _, err := pr.Read(buf); err == io.EOF {
			break
		}
	}

	var last ProgressUpdate
	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	for scanner.Scan() {
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &last))
	}
	require.Equal(t, int64(5120), last.CurrentBytes)
}
