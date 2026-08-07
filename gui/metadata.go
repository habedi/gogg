package gui

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
)

// metadataFetchers bounds how many games are looked up at once. Metadata is
// fetched for the game you are looking at, so this only matters when clicking
// quickly through a library.
const metadataFetchers = 2

// metadataMaxAge is how long a stored description is trusted. Store copy
// changes rarely, and a stale summary is better than none.
const metadataMaxAge = 30 * 24 * time.Hour

// metadataFormat is the shape gogg writes a lookup in. What gogg reads out of
// GOG's answer grows: a record written before it read screenshots says nothing
// about screenshots, which is not the same as a game having none. Raising this
// makes every older record be looked up again. 3 added the description with
// its markup kept.
const metadataFormat = 3

// metadataCache keeps GOG's store information for a game, in memory for this
// session and in the catalogue database between runs: it is what gogg knows
// about the game, so it lives with the game.
type metadataCache struct {
	fetches chan struct{}

	// ctx is cancelled when the cache is closed, taking every lookup still in
	// flight with it: a library that has been replaced has no pane to fill
	// and no business writing to the database on its way out.
	ctx    context.Context
	cancel context.CancelFunc

	// memory is read and written by every lookup goroutine.
	mu     sync.Mutex
	memory map[int]client.GameMetadata
}

// close abandons the lookups still in flight.
func (c *metadataCache) close() { c.cancel() }

func (c *metadataCache) remembered(gameID int) (client.GameMetadata, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	meta, ok := c.memory[gameID]
	return meta, ok
}

func (c *metadataCache) remember(gameID int, meta client.GameMetadata) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memory[gameID] = meta
}

func newMetadataCache() *metadataCache {
	ctx, cancel := context.WithCancel(context.Background())
	return &metadataCache{
		fetches: make(chan struct{}, metadataFetchers),
		ctx:     ctx,
		cancel:  cancel,
		memory:  make(map[int]client.GameMetadata),
	}
}

// fetch returns a game's store information, from the database when it is there
// and still fresh, and from GOG otherwise.
func (c *metadataCache) fetch(ctx context.Context, gameID int) (client.GameMetadata, error) {
	if meta, ok := c.remembered(gameID); ok {
		return meta, nil
	}

	if meta, ok := c.stored(ctx, gameID); ok {
		c.remember(gameID, meta)
		return meta, nil
	}

	c.fetches <- struct{}{}
	meta, err := client.FetchGameMetadata(ctx, gameID)
	<-c.fetches
	if err != nil {
		return client.GameMetadata{}, err
	}

	c.store(ctx, gameID, meta)
	c.remember(gameID, meta)
	return meta, nil
}

// stored reads what an earlier lookup recorded, when it is still worth
// trusting: fresh enough, and written in the shape gogg now reads.
func (c *metadataCache) stored(ctx context.Context, gameID int) (client.GameMetadata, bool) {
	record, err := db.GetGameMetadata(ctx, gameID)
	if err != nil || record == nil {
		return client.GameMetadata{}, false
	}
	if record.Format != metadataFormat || time.Since(record.FetchedAt) >= metadataMaxAge {
		return client.GameMetadata{}, false
	}

	var meta client.GameMetadata
	if json.Unmarshal(record.Data, &meta) != nil {
		return client.GameMetadata{}, false
	}
	return meta, true
}

// store records a lookup for the runs to come. A lookup that cannot be
// recorded is still an answer, so failing to store is only logged.
func (c *metadataCache) store(ctx context.Context, gameID int, meta client.GameMetadata) {
	data, err := json.Marshal(meta)
	if err != nil {
		return
	}
	if err := db.PutGameMetadata(ctx, gameID, metadataFormat, data); err != nil {
		log.Debug().Err(err).Int("gameID", gameID).Msg("Could not store metadata")
	}
}

// load looks a game up off the UI thread and delivers on it. stillWanted is
// asked whether the answer is still for the game on screen: the user may have
// clicked on by the time GOG answers. failed, when given, is told that the
// lookup came back with nothing, under the same condition.
func (c *metadataCache) load(gameID int, stillWanted func(int) bool, deliver func(client.GameMetadata), failed func()) {
	go func() {
		ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
		defer cancel()

		meta, err := c.fetch(ctx, gameID)
		runOnMain(func() {
			if stillWanted != nil && !stillWanted(gameID) {
				return
			}
			if err != nil {
				log.Debug().Err(err).Int("gameID", gameID).Msg("No store information")
				if failed != nil {
					failed()
				}
				return
			}
			deliver(meta)
		})
	}()
}
