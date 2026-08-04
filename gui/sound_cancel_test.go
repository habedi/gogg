package gui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A new notification stops the one that is still playing. When the superseded
// sound finishes cleaning up, it must not deregister its successor.
func TestBeginSound_SupersedesPreviousSound(t *testing.T) {
	first, releaseFirst := beginSound()
	second, releaseSecond := beginSound()

	require.Error(t, first.Err(), "starting a sound stops the one still playing")
	require.NoError(t, second.Err())

	// The superseded sound tidies up late.
	releaseFirst()

	third, releaseThird := beginSound()
	require.Error(t, second.Err(), "the current sound must still be cancellable")
	require.NoError(t, third.Err())

	releaseThird()
	releaseSecond()
}

// Releasing the current sound leaves nothing behind to cancel.
func TestBeginSound_ReleaseRetiresRegistration(t *testing.T) {
	ctx, release := beginSound()
	release()
	require.Error(t, ctx.Err(), "release cancels its own context")

	currentSoundMux.Lock()
	registered := currentSound
	currentSoundMux.Unlock()
	require.Nil(t, registered)
}
