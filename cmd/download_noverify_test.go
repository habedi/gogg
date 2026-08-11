package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestDownloadCmd_NoVerifyDefaultsToVerifying(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // an empty config dir, so no config.json
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())

	flag := downloadCmd(auth.NewService(nil, nil)).Flags().Lookup("no-verify")
	require.NotNil(t, flag, "the download command offers --no-verify")
	require.Equal(t, "false", flag.DefValue, "verification stays on unless it is turned off")
}

func TestDownloadCmd_NoVerifyTakesItsDefaultFromTheConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)

	path, err := config.Path()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	data, err := json.Marshal(map[string]any{"no_verify": true})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))

	flag := downloadCmd(auth.NewService(nil, nil)).Flags().Lookup("no-verify")
	require.NotNil(t, flag)
	require.Equal(t, "true", flag.DefValue)
}

// A config file written before the key existed must keep verification on.
func TestConfigLoad_OlderConfigKeepsVerificationOn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)

	path, err := config.Path()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"language":"de","threads":3}`), 0o600))

	cfg := config.Load()
	require.Equal(t, "de", cfg.Language)
	require.False(t, cfg.NoVerify)
}
