package operations

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A non-positive thread count must not silently produce zero hashes.
func TestGenerateHashes_NonPositiveThreadCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, threads := range []int{0, -1} {
		var count int
		for res := range GenerateHashes(context.Background(), []string{path}, "md5", threads) {
			if res.Err != nil {
				t.Errorf("threads=%d: %v", threads, res.Err)
			}
			count++
		}
		if count != 1 {
			t.Errorf("threads=%d: got %d results, want 1", threads, count)
		}
	}
}
