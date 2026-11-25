package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func fetchInstallerSet(t *testing.T, root string, opts DownloadOptions, names []string, forbidden string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if forbidden != "" && filepath.Base(r.URL.Path) == forbidden {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Length", "4")
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, "data")
		}
	}))
	defer server.Close()
	var files []PlatformFile
	for _, name := range names {
		url := server.URL + "/" + name
		files = append(files, PlatformFile{Name: name, ManualURL: &url})
	}
	var platform Platform
	switch opts.Platform {
	case "mac":
		platform.Mac = files
	case "linux":
		platform.Linux = files
	default:
		platform.Windows = files
	}
	game := Game{Title: "Some Game", Downloads: []Downloadable{{Language: opts.Language, Platforms: platform}}}
	require.NoError(t, DownloadGameFiles(context.Background(), "token", game, root, opts, io.Discard))
}

func TestPruneCompletedInstallerSet(t *testing.T) {
	for _, tt := range []struct {
		name, platform, dir string
		old, new            []string
		romm, lutris        bool
	}{
		{"Windows multipart", "windows", "some-game/windows", []string{"setup_prey_12742273_(64bit)_(65935).exe", "setup_prey_12742273_(64bit)_(65935)-1.bin", "setup_prey_12742273_(64bit)_(65935)-2.bin"}, []string{"setup_prey_12742274_(64bit)_(65936).exe", "setup_prey_12742274_(64bit)_(65936)-1.bin"}, false, false},
		{"Linux", "linux", "linux/some-game", []string{"game_1.2.3_65935.sh"}, []string{"game_1.3.0_65936.sh"}, true, false},
		{"Mac", "mac", "some-game/gog", []string{"game_1.2.3_(65935).pkg"}, []string{"game_1.3.0_(65936).pkg"}, false, true},
		{"Archive", "linux", "some-game/linux", []string{"game_1.2.3.tar.gz"}, []string{"game_1.3.0.tar.gz"}, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			options := DownloadOptions{Language: "English", Platform: tt.platform, Threads: 2, RomMLayout: tt.romm, LutrisLayout: tt.lutris}
			fetchInstallerSet(t, root, options, tt.old, "")
			options.Cleanup = &InstallerCleanup{}
			fetchInstallerSet(t, root, options, tt.new, "")
			dir := filepath.Join(root, tt.dir)
			unmanaged := filepath.Join(dir, "game_1.0.0.tar.gz")
			require.NoError(t, os.WriteFile(unmanaged, []byte("mine"), 0600))
			removed, err := PruneOldInstallerVersions(root, "Some Game", options)
			require.NoError(t, err)
			require.Len(t, removed, len(tt.old))
			for _, name := range tt.old {
				require.NoFileExists(t, filepath.Join(dir, name))
			}
			for _, name := range tt.new {
				require.FileExists(t, filepath.Join(dir, name))
			}
			require.FileExists(t, unmanaged)
			entries, err := readFileManifest(filepath.Join(options.Cleanup.passes[0].dir, "files.json"))
			require.NoError(t, err)
			require.Len(t, entries, len(tt.new))
		})
	}
}

func TestPruneKeepsOldBuildUntilEveryNewPartArrives(t *testing.T) {
	root := t.TempDir()
	opts := DownloadOptions{Language: "English", Platform: "windows", Threads: 2, Resume: true}
	old := []string{"setup_game_1.0_(100).exe", "setup_game_1.0_(100)-1.bin"}
	newFiles := []string{"setup_game_2.0_(200).exe", "setup_game_2.0_(200)-1.bin"}
	fetchInstallerSet(t, root, opts, old, "")
	opts.Cleanup = &InstallerCleanup{}
	fetchInstallerSet(t, root, opts, newFiles, newFiles[1])
	removed, err := PruneOldInstallerVersions(root, "Some Game", opts)
	require.NoError(t, err)
	require.Empty(t, removed)
	for _, name := range old {
		require.FileExists(t, filepath.Join(root, "some-game", "windows", name))
	}
	opts.Cleanup = &InstallerCleanup{}
	fetchInstallerSet(t, root, opts, newFiles, "")
	removed, err = PruneOldInstallerVersions(root, "Some Game", opts)
	require.NoError(t, err)
	require.Len(t, removed, 2)
}

func TestPruneProtectsModifiedAndOtherLanguageFiles(t *testing.T) {
	root := t.TempDir()
	opts := DownloadOptions{Language: "Deutsch", Platform: "mac", Threads: 1, Flatten: true}
	fetchInstallerSet(t, root, opts, []string{"game_1.0.dmg"}, "")
	opts.Language = "English"
	fetchInstallerSet(t, root, opts, []string{"game_1.1.dmg"}, "")
	require.NoError(t, os.WriteFile(filepath.Join(root, "some-game", "game_1.1.dmg"), []byte("mine"), 0600))
	opts.Cleanup = &InstallerCleanup{}
	fetchInstallerSet(t, root, opts, []string{"game_1.2.dmg"}, "")
	removed, err := PruneOldInstallerVersions(root, "Some Game", opts)
	require.NoError(t, err)
	require.Empty(t, removed)
	for _, name := range []string{"game_1.0.dmg", "game_1.1.dmg", "game_1.2.dmg"} {
		require.FileExists(t, filepath.Join(root, "some-game", name))
	}
}

func TestInstallerReplacementIdentity(t *testing.T) {
	old := DownloadedFile{Path: "game_1_2_3_45678.sh", Installer: &InstallerInfo{Language: "English", Platform: "linux", Product: "Game", Version: "1.2.3"}}
	fresh := DownloadedFile{Path: "game_1_2_4_45679.sh", Installer: &InstallerInfo{Language: "English", Platform: "linux", Product: "Game", Version: "1.2.4"}}
	require.True(t, supersedes(fresh, old))
	require.False(t, supersedes(old, fresh))
	old.Installer.DLC = true
	require.False(t, supersedes(fresh, old))

	fresh.Path = "windows/setup_game_english_2.0.exe"
	fresh.Installer.Platform = "windows"
	require.True(t, supersedes(fresh, DownloadedFile{Path: "windows/setup_game_english_1.0.exe"}))
	require.False(t, supersedes(fresh, DownloadedFile{Path: "windows/setup_game_english_2.0.0.exe"}))
	fresh.Path = "windows/setup_game_2.0.exe"
	require.False(t, supersedes(fresh, DownloadedFile{Path: "windows/setup_game_1.0.exe"}))
	fresh.Path = "setup_game_1.1_(64bit)_(110).exe"
	old.Path = "setup_game_1.0_(32bit)_(100).exe"
	old.Installer = fresh.Installer
	require.False(t, supersedes(fresh, old))
}

func TestPruneProtectsSharedInstallerAfterResume(t *testing.T) {
	root := t.TempDir()
	opts := DownloadOptions{Language: "English", Platform: "mac", Threads: 1, Flatten: true, Resume: true}
	fetchInstallerSet(t, root, opts, []string{"game_1.0.dmg"}, "")
	opts.Language = "Deutsch"
	fetchInstallerSet(t, root, opts, []string{"game_1.0.dmg"}, "")
	opts.Language = "English"
	opts.Cleanup = &InstallerCleanup{}
	fetchInstallerSet(t, root, opts, []string{"game_2.0.dmg"}, "")
	removed, err := PruneOldInstallerVersions(root, "Some Game", opts)
	require.NoError(t, err)
	require.Empty(t, removed)
	require.FileExists(t, filepath.Join(root, "some-game", "game_1.0.dmg"))
}
