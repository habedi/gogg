package gui

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

func TestGuiPruneOldVersions_RemovesOlderVersionsInSameFolder(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.2.3.exe",
		"some-game/windows/setup_game_1.2.10.exe",
	)

	if err := guiPruneOldVersions(root, "Some Game", false, "windows"); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root, "some-game/windows/setup_game_1.2.10.exe")
	assertGone(t, root, "some-game/windows/setup_game_1.2.3.exe")
}

func TestGuiPruneOldVersions_KeepsOtherPlatforms(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.2.3.exe",
		"some-game/linux/setup_game_1.2.4.sh",
	)

	if err := guiPruneOldVersions(root, "Some Game", false, "all"); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root,
		"some-game/windows/setup_game_1.2.3.exe",
		"some-game/linux/setup_game_1.2.4.sh",
	)
}

func TestGuiPruneOldVersions_KeepsOtherPlatformsWhenFlattened(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/setup_game_1.2.3.exe",
		"some-game/setup_game_1.2.4.sh",
	)

	if err := guiPruneOldVersions(root, "Some Game", false, "all"); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root, "some-game/setup_game_1.2.3.exe", "some-game/setup_game_1.2.4.sh")
}

func TestGuiPruneOldVersions_KeepsDLCInstallers(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"some-game/windows/setup_game_1.0.0.exe",
		"some-game/dlcs/the-dlc/windows/setup_game_2.0.0.exe",
	)

	if err := guiPruneOldVersions(root, "Some Game", false, "windows"); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root,
		"some-game/windows/setup_game_1.0.0.exe",
		"some-game/dlcs/the-dlc/windows/setup_game_2.0.0.exe",
	)
}

// In the RomM layout each platform has its own root; pruning one must not
// reach into another.
func TestGuiPruneOldVersions_RommLayoutKeepsEachPlatform(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"windows/some-game/setup_game_1.2.3.exe",
		"linux/some-game/setup_game_1.2.4.sh",
		"windows/some-game/setup_game_1.2.4.exe",
	)

	if err := guiPruneOldVersions(root, "Some Game", true, "all"); err != nil {
		t.Fatal(err)
	}

	assertExists(t, root,
		"windows/some-game/setup_game_1.2.4.exe",
		"linux/some-game/setup_game_1.2.4.sh",
	)
	assertGone(t, root, "windows/some-game/setup_game_1.2.3.exe")
}
