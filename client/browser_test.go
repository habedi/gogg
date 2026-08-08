package client

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func fakeBrowser(t *testing.T, name string) (dir, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX executable bits")
	}
	dir = t.TempDir()
	path = filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755))
	return dir, path
}

// Chrome and friends are called different things on different distributions;
// missing a name means gogg claims no browser is installed when one is.
func TestFindBrowser_KnowsDistributionSpecificNames(t *testing.T) {
	for _, name := range []string{
		"google-chrome", "google-chrome-stable",
		"chromium", "chromium-browser",
		"microsoft-edge", "msedge",
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := fakeBrowser(t, name)
			t.Setenv("GOGG_BROWSER", "")
			t.Setenv("PATH", dir)

			got, err := findBrowser()
			require.NoError(t, err)
			require.Equal(t, path, got)
		})
	}
}

// GOGG_BROWSER is the way out when the browser on PATH cannot be driven, as a
// snap-confined Chromium cannot.
func TestFindBrowser_GoggBrowserOverridesTheSearch(t *testing.T) {
	dir, path := fakeBrowser(t, "my-browser")
	otherDir, otherPath := fakeBrowser(t, "chromium")

	t.Setenv("PATH", otherDir)
	t.Setenv("GOGG_BROWSER", path)

	got, err := findBrowser()
	require.NoError(t, err)
	require.Equal(t, path, got, "the override wins over what is on PATH")
	require.NotEqual(t, otherPath, got)
	_ = dir
}

func TestFindBrowser_GoggBrowserAcceptsABareName(t *testing.T) {
	dir, path := fakeBrowser(t, "my-browser")
	t.Setenv("PATH", dir)
	t.Setenv("GOGG_BROWSER", "my-browser")

	got, err := findBrowser()
	require.NoError(t, err)
	require.Equal(t, path, got)
}

func TestFindBrowser_RejectsAnUnusableOverride(t *testing.T) {
	t.Setenv("GOGG_BROWSER", "/definitely/not/here")

	_, err := findBrowser()
	require.Error(t, err)
	require.Contains(t, err.Error(), "GOGG_BROWSER")
}

// With no browser at all, the message has to name both ways forward.
func TestFindBrowser_ErrorPointsAtTheWayOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX path handling")
	}
	t.Setenv("GOGG_BROWSER", "")
	t.Setenv("PATH", t.TempDir())

	_, err := findBrowser()
	require.Error(t, err)
	require.Contains(t, err.Error(), "GOGG_BROWSER")
	require.Contains(t, strings.ToLower(err.Error()), "--code")
}
