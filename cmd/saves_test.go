package cmd

import (
	"bytes"
	"testing"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func TestSavesCmd_InvalidID(t *testing.T) {
	authService := auth.NewService(nil, nil)
	cmd := savesCmd(authService)
	cmd.SetArgs([]string{"abc", t.TempDir()})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	_ = cmd.Execute()
	require.Contains(t, buf.String(), "Invalid game ID")
}

func TestSavesCmd_MissingDirNoConfig(t *testing.T) {
	// With outputDir omitted and no download_dir configured, the command must
	// say what is missing instead of guessing a destination for save files.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cleanDBTables(t)
	require.NoError(t, db.Db.Create(&db.Game{ID: 77, Title: "Some Game", Data: "{}"}).Error)

	authService := auth.NewService(nil, nil)
	cmd := savesCmd(authService)
	cmd.SetArgs([]string{"77"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	_ = cmd.Execute()
	require.Contains(t, buf.String(), "outputDir argument is required")
}

func TestSavesCmd_GameNotInCatalogue(t *testing.T) {
	cleanDBTables(t)
	authService := auth.NewService(nil, nil)
	cmd := savesCmd(authService)
	cmd.SetArgs([]string{"424242", t.TempDir()})
	out := captureStdout(func() { _ = cmd.Execute() })
	require.Contains(t, out, "not found in local catalogue")
}
