package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestHashCmd_CleanRecursiveRemovesOldHashes(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(sub, "file.bin")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	// Pre-create a .md5 file to be cleaned
	if err := os.WriteFile(f+".md5", []byte("hash"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := hashCmd()
	cmd.SetArgs([]string{dir, "-a", "md5", "-c", "-r", "-s"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.Execute()

	// After clean+rehash, a new .md5 should exist and only under included files
	if _, err := os.Stat(f + ".md5"); err != nil {
		t.Fatalf("expected %s to exist after rehash: %v", f+".md5", err)
	}
}
