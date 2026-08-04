package gui

import "fyne.io/fyne/v2/data/binding"

// catalogueUpdated is a simple data binding that acts as a signal bus.
// Any part of the UI can listen for changes to know when the catalogue is refreshed.
var catalogueUpdated = binding.NewBool()

// SignalCatalogueUpdated sends a notification that the catalogue has been updated.
// The value carries no meaning, only the change event, so it is toggled: a
// binding notifies its listeners only when the value actually changes.
func SignalCatalogueUpdated() {
	current, err := catalogueUpdated.Get()
	if err != nil {
		return
	}
	_ = catalogueUpdated.Set(!current)
}
