package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// What the disk refuses is as much a part of these helpers as what it allows,
// and every case below is a real file or a real directory rather than a stand-in
// for one.

func TestMD5OfFile_UnreadableFileLeavesTheChecksumUnrecorded(t *testing.T) {
	require.Empty(t, md5OfFile(filepath.Join(t.TempDir(), "was-never-written")),
		"a file that cannot be opened has no checksum to record")
}

func TestMD5OfFile_HashesWhatIsOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setup.exe")
	require.NoError(t, os.WriteFile(path, []byte("the real bytes"), 0o644))

	// printf 'the real bytes' | md5sum
	require.Equal(t, "4775f29afe5c07b856ff04f691139e43", md5OfFile(path))
}

func TestEnsureDirExists_RefusesAPathThatIsAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(path, []byte("in the way"), 0o644))

	err := ensureDirExists(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a directory")
}

func TestWriteFileManifest_KeepsWhatEarlierRunsBrought(t *testing.T) {
	path := filepath.Join(t.TempDir(), "files.json")
	writeFileManifest(path, []DownloadedFile{
		{Path: "windows/setup.exe", SizeBytes: 10, DownloadedAt: time.Now().UTC()},
	})
	writeFileManifest(path, []DownloadedFile{
		{Path: "windows/setup-1.bin", SizeBytes: 20, DownloadedAt: time.Now().UTC()},
	})

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entries []DownloadedFile
	require.NoError(t, json.Unmarshal(data, &entries))
	require.Len(t, entries, 2, "the file an earlier run brought stays in the manifest")
	require.Equal(t, "windows/setup-1.bin", entries[0].Path, "the entries are sorted by path")
}

func TestWriteFileManifest_ReplacesTheEntryForAFileBroughtAgain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "files.json")
	writeFileManifest(path, []DownloadedFile{{Path: "windows/setup.exe", SizeBytes: 10, MD5: "old"}})
	writeFileManifest(path, []DownloadedFile{{Path: "windows/setup.exe", SizeBytes: 20, MD5: "new"}})

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entries []DownloadedFile
	require.NoError(t, json.Unmarshal(data, &entries))
	require.Len(t, entries, 1)
	require.Equal(t, "new", entries[0].MD5)
	require.Equal(t, int64(20), entries[0].SizeBytes)
}

func TestWriteFileManifest_StartsOverOnAManifestItCannotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "files.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json at all"), 0o644))

	writeFileManifest(path, []DownloadedFile{{Path: "windows/setup.exe", SizeBytes: 10}})

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entries []DownloadedFile
	require.NoError(t, json.Unmarshal(data, &entries))
	require.Len(t, entries, 1, "an unreadable manifest is replaced rather than kept")
}

func TestWriteFileManifest_SaysNothingWhenTheDirectoryCannotBeMade(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "in-the-way")
	require.NoError(t, os.WriteFile(blocked, []byte("a file, not a directory"), 0o644))

	// The manifest would have to live inside a regular file, which cannot be.
	// Writing it fails, and the download it describes must not.
	writeFileManifest(filepath.Join(blocked, "files.json"), []DownloadedFile{{Path: "setup.exe"}})

	info, err := os.Stat(blocked)
	require.NoError(t, err)
	require.False(t, info.IsDir(), "the file that was in the way is left as it was")
}

func TestEnsureDirExists_RefusesADirectoryUnderAFile(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "in-the-way")
	require.NoError(t, os.WriteFile(blocked, []byte("a file, not a directory"), 0o644))

	require.Error(t, ensureDirExists(filepath.Join(blocked, "windows")),
		"no directory can be made inside a regular file")
}

func TestEnsureDirExists_MakesTheWholePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "god-of-war", "windows")
	require.NoError(t, ensureDirExists(path))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// Asking again for a directory that is already there is not an error.
	require.NoError(t, ensureDirExists(path))
}
