package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFiles creates every named file (relative to root) with dummy content.
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

// The point of --keep-latest: older installers in the same folder go away.
func TestPruneOldVersions_RemovesOlderVersionsInSameFolder(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.2.3.exe",
		"some-game/windows/setup_game_1.2.10.exe",
	)

	if err := pruneOldVersions(root, "Some Game"); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root, "some-game/windows/setup_game_1.2.10.exe")
	assertGone(t, root, "some-game/windows/setup_game_1.2.3.exe")
}

// Installers for different platforms are different files, not versions of one
// another, even when the version numbers differ.
func TestPruneOldVersions_KeepsOtherPlatforms(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.2.3.exe",
		"some-game/linux/setup_game_1.2.4.sh",
		"some-game/mac/setup_game_1.2.5.dmg",
	)

	if err := pruneOldVersions(root, "Some Game"); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root,
		"some-game/windows/setup_game_1.2.3.exe",
		"some-game/linux/setup_game_1.2.4.sh",
		"some-game/mac/setup_game_1.2.5.dmg",
	)
}

// With --flatten every platform lands in one folder, so the extension is what
// tells the files apart.
func TestPruneOldVersions_KeepsOtherPlatformsWhenFlattened(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/setup_game_1.2.3.exe",
		"some-game/setup_game_1.2.4.sh",
	)

	if err := pruneOldVersions(root, "Some Game"); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root, "some-game/setup_game_1.2.3.exe", "some-game/setup_game_1.2.4.sh")
}

// A DLC installer must not delete the base game's installer just because they
// share a file name prefix.
func TestPruneOldVersions_KeepsDLCInstallers(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.0.0.exe",
		"some-game/dlcs/the-dlc/windows/setup_game_2.0.0.exe",
	)

	if err := pruneOldVersions(root, "Some Game"); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root,
		"some-game/windows/setup_game_1.0.0.exe",
		"some-game/dlcs/the-dlc/windows/setup_game_2.0.0.exe",
	)
}

// Files that are not installers are never touched.
func TestPruneOldVersions_IgnoresNonInstallerFiles(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.0.0.exe",
		"some-game/windows/setup_game_2.0.0.exe",
		"some-game/metadata.json",
		"some-game/windows/manual_1.0.0.pdf",
	)

	if err := pruneOldVersions(root, "Some Game"); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root,
		"some-game/windows/setup_game_2.0.0.exe",
		"some-game/metadata.json",
		"some-game/windows/manual_1.0.0.pdf",
	)
	assertGone(t, root, "some-game/windows/setup_game_1.0.0.exe")
}
