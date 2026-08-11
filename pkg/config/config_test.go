package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolatedConfigPath points the config file at a temporary location on every
// platform and returns that path. os.UserConfigDir, which Path builds on,
// reads XDG_CONFIG_HOME on Linux, HOME/Library/Application Support on macOS,
// and %AppData% on Windows, so all three are redirected. The earlier tests
// set only XDG_CONFIG_HOME and wrote to a hardcoded "gogg" folder, which
// passed on Linux but read the wrong location on macOS and Windows.
func isolatedConfigPath(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
	t.Setenv("AppData", tmp)
	p, err := Path()
	require.NoError(t, err)
	return p
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	assert.Equal(t, "en", cfg.Language)
	assert.Equal(t, "windows", cfg.Platform)
	assert.True(t, cfg.Extras)
	assert.True(t, cfg.DLCs)
	assert.True(t, cfg.Resume)
	assert.Equal(t, 5, cfg.Threads)
	assert.True(t, cfg.Flatten)
	assert.True(t, cfg.SkipPatches, "patches are skipped by default, the same as the GUI")
	assert.False(t, cfg.KeepLatest)
	assert.False(t, cfg.RommLayout)
}

func TestLoad_NoFile(t *testing.T) {
	isolatedConfigPath(t)
	cfg := Load()
	// Should return defaults when file doesn't exist.
	assert.Equal(t, Defaults(), cfg)
}

func TestLoad_ValidFile(t *testing.T) {
	p := isolatedConfigPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
	data := `{"language":"fr","platform":"linux","extras":false,"dlcs":false,"threads":3}`
	require.NoError(t, os.WriteFile(p, []byte(data), 0o600))

	cfg := Load()
	assert.Equal(t, "fr", cfg.Language)
	assert.Equal(t, "linux", cfg.Platform)
	assert.False(t, cfg.Extras)
	assert.False(t, cfg.DLCs)
	assert.Equal(t, 3, cfg.Threads)
	// Fields not set in JSON fall back to defaults.
	assert.True(t, cfg.Resume)
	assert.True(t, cfg.Flatten)
}

func TestLoad_InvalidJSON(t *testing.T) {
	p := isolatedConfigPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
	require.NoError(t, os.WriteFile(p, []byte("not json"), 0o600))

	cfg := Load()
	assert.Equal(t, Defaults(), cfg)
}

func TestSave_RoundTrip(t *testing.T) {
	isolatedConfigPath(t)

	want := Config{
		Language:    "de",
		Platform:    "mac",
		DownloadDir: "/data/games",
		Extras:      false,
		DLCs:        true,
		Resume:      false,
		Threads:     8,
		Flatten:     false,
		SkipPatches: true,
		KeepLatest:  true,
		RommLayout:  true,
	}
	require.NoError(t, Save(want))

	p, err := Path()
	require.NoError(t, err)
	data, err := os.ReadFile(p)
	require.NoError(t, err)

	var got Config
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, want, got)

	// Load should reproduce the same config.
	loaded := Load()
	assert.Equal(t, want, loaded)
}

func TestSave_CreatesDir(t *testing.T) {
	isolatedConfigPath(t)
	// The config directory does not exist yet; Save must create it.
	require.NoError(t, Save(Defaults()))
	p, err := Path()
	require.NoError(t, err)
	assert.FileExists(t, p)
}

func TestSave_ReportsADirectoryItCannotCreate(t *testing.T) {
	dir := t.TempDir()
	// A regular file where the config directory has to go: MkdirAll cannot
	// make a directory out of it, so Save has nowhere to write.
	blocked := filepath.Join(dir, "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("in the way"), 0o600))
	t.Setenv("XDG_CONFIG_HOME", blocked)
	t.Setenv("HOME", blocked)
	t.Setenv("AppData", blocked)

	require.Error(t, Save(Defaults()))
}

func TestLoad_IgnoresAConfigItCannotParse(t *testing.T) {
	p := isolatedConfigPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
	require.NoError(t, os.WriteFile(p, []byte("{not json at all"), 0o600))

	// A config file that cannot be read falls back to the built-in defaults
	// rather than leaving the command with nothing.
	assert.Equal(t, Defaults(), Load())
}

func TestPath_ReportsAHomelessEnvironment(t *testing.T) {
	// os.UserConfigDir reads XDG_CONFIG_HOME then HOME on Linux, HOME on
	// macOS, and AppData on Windows. With all of them empty it has nowhere
	// to point, and everything built on it says so rather than guessing.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("AppData", "")

	if _, err := Path(); err == nil {
		t.Skip("this platform still has a config directory without those variables")
	}

	require.Error(t, Save(Defaults()))
	assert.Equal(t, Defaults(), Load(), "a config that cannot be found leaves the defaults")
}
