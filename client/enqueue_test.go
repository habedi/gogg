package client

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- enqueueExtras ---

func TestEnqueueExtras_Empty(t *testing.T) {
	var tasks []downloadTask
	err := enqueueExtras(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, nil, "extras", false, false)
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestEnqueueExtras_SkipsEmptyManualURL(t *testing.T) {
	extras := []Extra{{Name: "soundtrack", ManualURL: ""}}
	var tasks []downloadTask
	err := enqueueExtras(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, extras, "extras", false, false)
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestEnqueueExtras_EnqueuesTaskWithExtension(t *testing.T) {
	extras := []Extra{
		{Name: "Soundtrack", ManualURL: "/extras/soundtrack.zip"},
		{Name: "Wallpaper", ManualURL: "https://cdn.gog.com/extras/wallpaper.jpg"},
	}
	var tasks []downloadTask
	err := enqueueExtras(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, extras, "extras", true, false)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "extras", tasks[0].subDir)
	assert.True(t, tasks[0].resume)
	assert.Equal(t, ".zip", filepath.Ext(tasks[0].fileName))
	assert.Equal(t, ".jpg", filepath.Ext(tasks[1].fileName))
}

func TestEnqueueExtras_FlattenFlag(t *testing.T) {
	extras := []Extra{{Name: "art", ManualURL: "/extras/art.png"}}
	var tasks []downloadTask
	err := enqueueExtras(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, extras, "extras", false, true)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.True(t, tasks[0].flatten)
}

func TestEnqueueExtras_ContextCancelled(t *testing.T) {
	extras := []Extra{{Name: "X", ManualURL: "/extras/x.zip"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := enqueueExtras(ctx, func(_ downloadTask) {}, extras, "extras", false, false)
	assert.ErrorIs(t, err, context.Canceled)
}

// --- enqueueDLCs ---

func TestEnqueueDLCs_Empty(t *testing.T) {
	var tasks []downloadTask
	err := enqueueDLCs(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, &Game{}, "en", "windows", false, false, false, false)
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestEnqueueDLCs_EnqueuesDLCGameFiles(t *testing.T) {
	dlcURL := "/dlc/setup.exe"
	game := &Game{
		DLCs: []DLC{
			{
				Title: "My DLC",
				ParsedDownloads: []Downloadable{
					{
						Language: "en",
						Platforms: Platform{
							Windows: []PlatformFile{{ManualURL: &dlcURL, Name: "setup.exe", Size: "1 GB"}},
						},
					},
				},
			},
		},
	}
	var tasks []downloadTask
	err := enqueueDLCs(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, game, "en", "windows", false, false, false, false)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Contains(t, tasks[0].subDir, "my-dlc")
	assert.Equal(t, "setup.exe", tasks[0].fileName)
}

func TestEnqueueDLCs_EnqueuesDLCExtrasWhenFlagSet(t *testing.T) {
	game := &Game{
		DLCs: []DLC{
			{
				Title:  "My DLC",
				Extras: []Extra{{Name: "art", ManualURL: "/extras/art.zip"}},
			},
		},
	}
	var tasks []downloadTask
	err := enqueueDLCs(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, game, "en", "windows", true, false, false, false)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Contains(t, tasks[0].subDir, "extras")
}

func TestEnqueueDLCs_SkipsDLCExtrasWhenFlagUnset(t *testing.T) {
	game := &Game{
		DLCs: []DLC{
			{
				Title:  "My DLC",
				Extras: []Extra{{Name: "art", ManualURL: "/extras/art.zip"}},
			},
		},
	}
	var tasks []downloadTask
	err := enqueueDLCs(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, game, "en", "windows", false, false, false, false)
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestEnqueueDLCs_MultipleDLCs(t *testing.T) {
	dlcURL1 := "/dlc1/setup.exe"
	dlcURL2 := "/dlc2/setup.exe"
	game := &Game{
		DLCs: []DLC{
			{
				Title: "DLC One",
				ParsedDownloads: []Downloadable{
					{Language: "en", Platforms: Platform{Windows: []PlatformFile{{ManualURL: &dlcURL1, Name: "dlc1.exe"}}}},
				},
			},
			{
				Title: "DLC Two",
				ParsedDownloads: []Downloadable{
					{Language: "en", Platforms: Platform{Windows: []PlatformFile{{ManualURL: &dlcURL2, Name: "dlc2.exe"}}}},
				},
			},
		},
	}
	var tasks []downloadTask
	err := enqueueDLCs(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, game, "en", "windows", false, false, false, false)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

// --- enqueueGameFiles ---

func TestEnqueueGameFiles_LanguageMismatch(t *testing.T) {
	url := "/win/setup.exe"
	game := Game{Downloads: []Downloadable{{
		Language:  "fr",
		Platforms: Platform{Windows: []PlatformFile{{ManualURL: &url, Name: "setup.exe"}}},
	}}}
	var tasks []downloadTask
	err := enqueueGameFiles(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, game, "en", "windows", "", false, false, false)
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestEnqueueGameFiles_NilManualURL(t *testing.T) {
	game := Game{Downloads: []Downloadable{{
		Language:  "en",
		Platforms: Platform{Windows: []PlatformFile{{ManualURL: nil, Name: "setup.exe"}}},
	}}}
	var tasks []downloadTask
	err := enqueueGameFiles(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, game, "en", "windows", "", false, false, false)
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestEnqueueGameFiles_AllPlatforms(t *testing.T) {
	winURL := "/win/setup.exe"
	linURL := "/lin/setup.sh"
	macURL := "/mac/setup.dmg"
	game := Game{Downloads: []Downloadable{{
		Language: "en",
		Platforms: Platform{
			Windows: []PlatformFile{{ManualURL: &winURL, Name: "setup.exe"}},
			Linux:   []PlatformFile{{ManualURL: &linURL, Name: "setup.sh"}},
			Mac:     []PlatformFile{{ManualURL: &macURL, Name: "setup.dmg"}},
		},
	}}}
	var tasks []downloadTask
	err := enqueueGameFiles(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, game, "en", "all", "", false, false, false)
	require.NoError(t, err)
	assert.Len(t, tasks, 3)
}

func TestEnqueueGameFiles_SkipPatchesByURL(t *testing.T) {
	patchURL := "/files/patch_1.0.exe"
	setupURL := "/files/setup.exe"
	game := Game{Downloads: []Downloadable{{
		Language: "en",
		Platforms: Platform{Windows: []PlatformFile{
			{ManualURL: &patchURL, Name: "patch_1.0.exe"},
			{ManualURL: &setupURL, Name: "setup.exe"},
		}},
	}}}
	var tasks []downloadTask
	err := enqueueGameFiles(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, game, "en", "windows", "", false, false, true)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "setup.exe", tasks[0].fileName)
}

func TestEnqueueGameFiles_SkipPatchesByName(t *testing.T) {
	patchURL := "/files/file1.exe"
	setupURL := "/files/setup.exe"
	game := Game{Downloads: []Downloadable{{
		Language: "en",
		Platforms: Platform{Windows: []PlatformFile{
			{ManualURL: &patchURL, Name: "patch_v2.exe"}, // "patch" in the name
			{ManualURL: &setupURL, Name: "setup.exe"},
		}},
	}}}
	var tasks []downloadTask
	err := enqueueGameFiles(context.Background(), func(t downloadTask) { tasks = append(tasks, t) }, game, "en", "windows", "", false, false, true)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "setup.exe", tasks[0].fileName)
}

func TestEnqueueGameFiles_ContextCancelled(t *testing.T) {
	url := "/win/setup.exe"
	game := Game{Downloads: []Downloadable{{
		Language:  "en",
		Platforms: Platform{Windows: []PlatformFile{{ManualURL: &url, Name: "setup.exe"}}},
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := enqueueGameFiles(ctx, func(_ downloadTask) {}, game, "en", "windows", "", false, false, false)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestEnqueueDLCs_ExtrasContextCancelled(t *testing.T) {
	// DLC has no game files (enqueueGameFiles returns nil immediately without
	// checking ctx) but has an extra. With a pre-cancelled context, enqueueExtras
	// should detect the cancellation and return context.Canceled.
	game := &Game{
		DLCs: []DLC{{
			Title:  "My DLC",
			Extras: []Extra{{Name: "art", ManualURL: "/extras/art.zip"}},
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := enqueueDLCs(ctx, func(_ downloadTask) {}, game, "en", "windows", true, false, false, false)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestEnqueueDLCs_ContextCancelled(t *testing.T) {
	cancelURL := "/dlc/setup.exe"
	game := &Game{
		DLCs: []DLC{
			{
				Title: "My DLC",
				ParsedDownloads: []Downloadable{
					{Language: "en", Platforms: Platform{Windows: []PlatformFile{{ManualURL: &cancelURL, Name: "setup.exe"}}}},
				},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := enqueueDLCs(ctx, func(_ downloadTask) {}, game, "en", "windows", false, false, false, false)
	assert.ErrorIs(t, err, context.Canceled)
}
