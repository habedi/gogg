package gui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// An estimation run keeps reading its inputs long after it was started, while
// the UI thread carries on filtering the list and reloading the catalogue.
func TestSnapshotEstimationInputs_IsolatedFromLaterMutation(t *testing.T) {
	titles := []string{"Alpha", "Beta"}
	ids := map[string]int{"Alpha": 1, "Beta": 2}

	gotTitles, gotIDs := snapshotEstimationInputs(titles, ids)

	// The UI carries on changing its own state.
	titles[0] = "Changed"
	titles = append(titles, "Gamma")
	ids["Alpha"] = 99
	delete(ids, "Beta")
	require.Len(t, titles, 3)
	require.Equal(t, 99, ids["Alpha"])

	require.Equal(t, []string{"Alpha", "Beta"}, gotTitles)
	require.Equal(t, map[string]int{"Alpha": 1, "Beta": 2}, gotIDs)
}

// Reading the snapshot while the originals are rebuilt must be race free.
// Run with -race.
func TestSnapshotEstimationInputs_SafeToReadWhileOriginalsChange(t *testing.T) {
	titles := []string{"Alpha", "Beta"}
	ids := map[string]int{"Alpha": 1, "Beta": 2}

	gotTitles, gotIDs := snapshotEstimationInputs(titles, ids)

	lookups := make(chan int, 1)
	go func() {
		found := 0
		for i := 0; i < 500; i++ {
			for _, title := range gotTitles {
				found += gotIDs[title]
			}
		}
		lookups <- found
	}()

	// Meanwhile the UI rebuilds its own copies, as refreshGameList does.
	rebuilt := 0
	for i := 0; i < 500; i++ {
		titles = []string{"Other"}
		ids = map[string]int{"Other": i}
		rebuilt += len(titles) + len(ids)
	}

	require.Equal(t, 1000, rebuilt)
	require.Equal(t, 1500, <-lookups)
}
