package gui

import "github.com/habedi/gogg/db"

// stores is every way the GUI reaches the database, handed in at the top so
// nothing below it has to touch the global handle.
type stores struct {
	games    db.GameRepository
	tokens   db.TokenRepository
	tags     db.TagRepository
	metadata db.MetadataRepository
}

// openStores builds the repositories over the connection main opened. This is
// the one place the GUI reads the global handle.
func openStores() stores {
	handle := db.GetDB()
	return stores{
		games:    db.NewGameRepository(handle),
		tokens:   db.NewTokenRepository(handle),
		tags:     db.NewTagRepository(handle),
		metadata: db.NewMetadataRepository(handle),
	}
}
