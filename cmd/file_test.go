package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/habedi/gogg/db"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestFileHashCmd_PrintsHashesAndSavesToFiles(t *testing.T) {
	dir := t.TempDir()
	fExcluded := filepath.Join(dir, "a.txt") // excluded by DefaultHashExclusions
	fIncluded := filepath.Join(dir, "b.bin") // included
	if err := os.WriteFile(fExcluded, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fIncluded, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := hashCmd()
	cmd.SetArgs([]string{dir, "-a", "md5", "-s", "-r"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.Execute()

	// Included file should have a hash file
	if _, err := os.Stat(fIncluded + ".md5"); err != nil {
		t.Fatalf("expected %s to exist: %v", fIncluded+".md5", err)
	}
	// Excluded file should not have a hash file
	if _, err := os.Stat(fExcluded + ".md5"); err == nil {
		t.Fatalf("did not expect %s to exist", fExcluded+".md5")
	}
}

func TestFileHashCmd_InvalidAlgo(t *testing.T) {
	cmd := hashCmd()
	cmd.SetArgs([]string{"/does/not/matter", "-a", "bad"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.Execute()
}

func setupMemDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Db = gormDB
	if err := db.Db.AutoMigrate(&db.Game{}, &db.Token{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// captureStdout runs f while capturing os.Stdout and returns captured output.
func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	_ = r.Close()
	return string(out)
}

func TestSizeCmd_HappyPathAndUnits(t *testing.T) {
	setupMemDB(t)
	raw := `{"title":"CLI Size Game","downloads":[["English", {"windows":[{"name":"setup.exe","size":"1 MB"}]}]],"extras":[],"dlcs":[]}`
	if err := db.PutInGame(991, "CLI Size Game", raw); err != nil {
		t.Skipf("skipping: %v", err)
	}

	for _, unit := range []string{"gb", "mb", "kb", "b"} {
		cmd := sizeCmd()
		cmd.SetArgs([]string{"991", "--lang", "en", "--platform", "windows", "--unit", unit})
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		out := captureStdout(func() { cmd.Execute() })
		if !strings.Contains(out, "Total download size:") {
			t.Fatalf("expected size output for unit %s, got: %s", unit, out)
		}
	}
}
