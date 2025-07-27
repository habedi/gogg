package gui

import "fyne.io/fyne/v2/data/binding"

// catalogueUpdated is a simple data binding that acts as a signal bus.
// Any part of the UI can listen for changes to know when the catalogue is refreshed.
var catalogueUpdated = binding.NewBool()

// SignalCatalogueUpdated sends a notification that the catalogue has been updated.
func SignalCatalogueUpdated() {
	err := catalogueUpdated.Set(true)
	if err != nil {
		return
	} // The value doesn't matter, only the change event.
}
