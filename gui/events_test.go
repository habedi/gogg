package gui

import (
	"testing"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
)

// The catalogue signal carries no value, only the event, so every refresh has
// to reach the listeners. A binding only notifies when its value changes.
func TestSignalCatalogueUpdated_NotifiesOnEverySignal(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	var fires int
	catalogueUpdated.AddListener(binding.NewDataListener(func() { fires++ }))
	afterRegistration := fires

	SignalCatalogueUpdated()
	require.Equal(t, afterRegistration+1, fires, "the first refresh must notify")

	SignalCatalogueUpdated()
	require.Equal(t, afterRegistration+2, fires, "later refreshes must notify too")

	SignalCatalogueUpdated()
	require.Equal(t, afterRegistration+3, fires)
}
