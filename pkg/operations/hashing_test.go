package operations_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/habedi/gogg/pkg/operations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create root files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "root.txt"), []byte("root"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.csv"), []byte("ignore"), 0600))

	// Create subdir
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "sub.txt"), []byte("sub"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "sub.txt.md5"), []byte("hash"), 0600))

	return dir
}

func TestFindFilesToHash(t *testing.T) {
	dir := createTestDir(t)
	testExclusions := []string{"*.csv", "*.md5"}

	t.Run("Recursive", func(t *testing.T) {
		files, err := operations.FindFilesToHash(dir, true, testExclusions)
		require.NoError(t, err)

		expected := []string{filepath.Join(dir, "root.txt"), filepath.Join(dir, "subdir", "sub.txt")}
		sort.Strings(files)
		sort.Strings(expected)

		assert.Equal(t, expected, files)
	})

	t.Run("Non-Recursive", func(t *testing.T) {
		files, err := operations.FindFilesToHash(dir, false, testExclusions)
		require.NoError(t, err)
		assert.Equal(t, []string{filepath.Join(dir, "root.txt")}, files)
	})

	t.Run("Non-existent dir", func(t *testing.T) {
		_, err := operations.FindFilesToHash("nonexistent-dir", true, nil)
		assert.Error(t, err)
	})
}

func TestGenerateHashes(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("gogg-test"), 0600))

	files := []string{filePath}
	resultsChan := operations.GenerateHashes(context.Background(), files, "md5", 1)

	result := <-resultsChan
	assert.NoError(t, result.Err)
	assert.Equal(t, filePath, result.File)
	assert.Equal(t, "3b121c2528589133486a8367417573f0", result.Hash)

	_, ok := <-resultsChan
	assert.False(t, ok, "Channel should be closed")
}

func TestCleanHashes(t *testing.T) {
	dir := createTestDir(t)

	err := operations.CleanHashes(dir, true)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "subdir", "sub.txt.md5"))
	assert.True(t, os.IsNotExist(err), "Hash file should have been deleted")

	_, err = os.Stat(filepath.Join(dir, "subdir", "sub.txt"))
	assert.NoError(t, err, "Regular file should still exist")
}
