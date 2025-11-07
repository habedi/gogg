package gui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAudioFile(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
		errMsg  string
	}{
		{
			name: "empty path",
			setup: func(t *testing.T) string {
				return ""
			},
			wantErr: true,
			errMsg:  "empty file path",
		},
		{
			name: "non-existent file",
			setup: func(t *testing.T) string {
				return "/nonexistent/path/to/file.mp3"
			},
			wantErr: true,
			errMsg:  "cannot access file",
		},
		{
			name: "directory instead of file",
			setup: func(t *testing.T) string {
				dir, err := os.MkdirTemp("", "audio-test-*")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.RemoveAll(dir) })
				return dir
			},
			wantErr: true,
			errMsg:  "path is a directory",
		},
		{
			name: "empty file",
			setup: func(t *testing.T) string {
				f, err := os.CreateTemp("", "audio-test-*.mp3")
				if err != nil {
					t.Fatal(err)
				}
				path := f.Name()
				f.Close()
				t.Cleanup(func() { os.Remove(path) })
				return path
			},
			wantErr: true,
			errMsg:  "file is empty",
		},
		{
			name: "file too large",
			setup: func(t *testing.T) string {
				f, err := os.CreateTemp("", "audio-test-*.mp3")
				if err != nil {
					t.Fatal(err)
				}
				path := f.Name()

				data := make([]byte, 51*1024*1024)
				if _, err := f.Write(data); err != nil {
					f.Close()
					os.Remove(path)
					t.Fatal(err)
				}
				f.Close()
				t.Cleanup(func() { os.Remove(path) })
				return path
			},
			wantErr: true,
			errMsg:  "file is too large",
		},
		{
			name: "unsupported format",
			setup: func(t *testing.T) string {
				f, err := os.CreateTemp("", "audio-test-*.mp4")
				if err != nil {
					t.Fatal(err)
				}
				path := f.Name()
				f.Write([]byte("fake content"))
				f.Close()
				t.Cleanup(func() { os.Remove(path) })
				return path
			},
			wantErr: true,
			errMsg:  "unsupported file format",
		},
		{
			name: "valid mp3 file",
			setup: func(t *testing.T) string {
				f, err := os.CreateTemp("", "audio-test-*.mp3")
				if err != nil {
					t.Fatal(err)
				}
				path := f.Name()
				f.Write([]byte("fake mp3 content that is not empty"))
				f.Close()
				t.Cleanup(func() { os.Remove(path) })
				return path
			},
			wantErr: false,
		},
		{
			name: "valid wav file",
			setup: func(t *testing.T) string {
				f, err := os.CreateTemp("", "audio-test-*.wav")
				if err != nil {
					t.Fatal(err)
				}
				path := f.Name()
				f.Write([]byte("fake wav content that is not empty"))
				f.Close()
				t.Cleanup(func() { os.Remove(path) })
				return path
			},
			wantErr: false,
		},
		{
			name: "valid ogg file",
			setup: func(t *testing.T) string {
				f, err := os.CreateTemp("", "audio-test-*.ogg")
				if err != nil {
					t.Fatal(err)
				}
				path := f.Name()
				f.Write([]byte("fake ogg content that is not empty"))
				f.Close()
				t.Cleanup(func() { os.Remove(path) })
				return path
			},
			wantErr: false,
		},
		{
			name: "uppercase extension",
			setup: func(t *testing.T) string {
				f, err := os.CreateTemp("", "audio-test-*.MP3")
				if err != nil {
					t.Fatal(err)
				}
				path := f.Name()
				f.Write([]byte("fake mp3 content"))
				f.Close()
				t.Cleanup(func() { os.Remove(path) })
				return path
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			err := validateAudioFile(path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateAudioFile() expected error but got none")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateAudioFile() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateAudioFile() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateAudioFile_RealFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires real audio files")
	}

	testdataDir := filepath.Join("testdata", "audio")
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		t.Skip("testdata/audio directory not found")
	}

	tests := []struct {
		filename string
		wantErr  bool
	}{
		{"valid.mp3", false},
		{"valid.wav", false},
		{"valid.ogg", false},
		{"corrupted.ogg", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			path := filepath.Join(testdataDir, tt.filename)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("Test file %s not found", path)
			}

			err := validateAudioFile(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAudioFile(%s) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAny(s, substr))
}

func containsAny(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
