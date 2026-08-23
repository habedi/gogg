package client

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func assertExists(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Errorf("expected %s to be kept: %v", name, err)
		}
	}
}

func assertGone(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err == nil {
			t.Errorf("expected %s to be removed", name)
		}
	}
}

func TestPruneOldInstallerVersions_RemovesOlderVersionsInSameFolder(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.2.3.exe",
		"some-game/windows/setup_game_1.2.10.exe",
	)

	if _, err := PruneOldInstallerVersions(root, "Some Game", DownloadOptions{Platform: "windows"}); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root, "some-game/windows/setup_game_1.2.10.exe")
	assertGone(t, root, "some-game/windows/setup_game_1.2.3.exe")
}

func TestPruneOldInstallerVersions_KeepsOtherPlatforms(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.2.3.exe",
		"some-game/linux/setup_game_1.2.4.sh",
	)

	if _, err := PruneOldInstallerVersions(root, "Some Game", DownloadOptions{Platform: "all"}); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root,
		"some-game/windows/setup_game_1.2.3.exe",
		"some-game/linux/setup_game_1.2.4.sh",
	)
}

func TestPruneOldInstallerVersions_KeepsOtherPlatformsWhenFlattened(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/setup_game_1.2.3.exe",
		"some-game/setup_game_1.2.4.sh",
	)

	if _, err := PruneOldInstallerVersions(root, "Some Game", DownloadOptions{Platform: "all"}); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root, "some-game/setup_game_1.2.3.exe", "some-game/setup_game_1.2.4.sh")
}

func TestPruneOldInstallerVersions_KeepsDLCInstallers(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.0.0.exe",
		"some-game/dlcs/the-dlc/windows/setup_game_2.0.0.exe",
	)

	if _, err := PruneOldInstallerVersions(root, "Some Game", DownloadOptions{Platform: "windows"}); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root,
		"some-game/windows/setup_game_1.0.0.exe",
		"some-game/dlcs/the-dlc/windows/setup_game_2.0.0.exe",
	)
}

// In the RomM layout each platform has its own root; pruning one must not
// reach into another.
func TestPruneOldInstallerVersions_RommLayoutKeepsEachPlatform(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"win/some-game/setup_game_1.2.3.exe",
		"linux/some-game/setup_game_1.2.4.sh",
		"win/some-game/setup_game_1.2.4.exe",
	)

	if _, err := PruneOldInstallerVersions(root, "Some Game", DownloadOptions{RomMLayout: true, Platform: "all"}); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root,
		"win/some-game/setup_game_1.2.4.exe",
		"linux/some-game/setup_game_1.2.4.sh",
	)
	assertGone(t, root, "win/some-game/setup_game_1.2.3.exe")
}

func TestPruneOldInstallerVersions_ReportsWhatItRemoved(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/setup_game_1.0.0.exe",
		"some-game/setup_game_2.0.0.exe",
	)

	removed, err := PruneOldInstallerVersions(root, "Some Game", DownloadOptions{Platform: "windows"})
	if err != nil {
		t.Fatal(err)
	}

	if len(removed) != 1 || filepath.Base(removed[0]) != "setup_game_1.0.0.exe" {
		t.Errorf("expected the report to name the removed file, got %v", removed)
	}
	assertExists(t, root, "some-game/setup_game_2.0.0.exe")
}

// Files that are not installers are never touched.
func TestPruneOldInstallerVersions_IgnoresNonInstallerFiles(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.0.0.exe",
		"some-game/windows/setup_game_2.0.0.exe",
		"some-game/metadata.json",
		"some-game/windows/manual_1.0.0.pdf",
	)

	if _, err := PruneOldInstallerVersions(root, "Some Game", DownloadOptions{Platform: "windows"}); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root,
		"some-game/windows/setup_game_2.0.0.exe",
		"some-game/metadata.json",
		"some-game/windows/manual_1.0.0.pdf",
	)
	assertGone(t, root, "some-game/windows/setup_game_1.0.0.exe")
}
