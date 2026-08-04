package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/habedi/gogg/client"
	"github.com/rs/zerolog/log"
)

// metadataFetchers bounds how many games are looked up at once. Metadata is
// fetched for the game you are looking at, so this only matters when clicking
// quickly through a library.
const metadataFetchers = 2

// metadataMaxAge is how long a stored description is trusted. Store copy
// changes rarely, and a stale summary is better than none.
const metadataMaxAge = 30 * 24 * time.Hour

// metadataCache keeps GOG's store information for a game, in memory for this
// session and on disk between runs.
type metadataCache struct {
	dir     string
	fetches chan struct{}

	// memory is read and written by every lookup goroutine.
	mu     sync.Mutex
	memory map[int]client.GameMetadata
}

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

func newMetadataCache(dir string) *metadataCache {
	return &metadataCache{
		dir:     dir,
		fetches: make(chan struct{}, metadataFetchers),
		memory:  make(map[int]client.GameMetadata),
	}
}

// metadataCacheDir is where descriptions are kept between runs.
func metadataCacheDir() string {
	return filepath.Join(cacheRoot(), "metadata")
}

// fetch returns a game's store information, from disk when it is there and
// still fresh, and from GOG otherwise.
func (c *metadataCache) fetch(ctx context.Context, gameID int) (client.GameMetadata, error) {
	if meta, ok := c.remembered(gameID); ok {
		return meta, nil
	}

	path := c.pathFor(gameID)
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < metadataMaxAge {
		if data, err := os.ReadFile(path); err == nil {
			var meta client.GameMetadata
			if json.Unmarshal(data, &meta) == nil {
				c.remember(gameID, meta)
				return meta, nil
			}
		}
	}

	c.fetches <- struct{}{}
	meta, err := client.FetchGameMetadata(ctx, gameID)
	<-c.fetches
	if err != nil {
		return client.GameMetadata{}, err
	}

	c.store(path, meta)
	c.remember(gameID, meta)
	return meta, nil
}

func (c *metadataCache) store(path string, meta client.GameMetadata) {
	data, err := json.Marshal(meta)
	if err != nil {
		return
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		log.Debug().Err(err).Msg("Could not create the metadata cache directory")
		return
	}
	temp := path + ".part"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		log.Debug().Err(err).Msg("Could not cache metadata")
		return
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
	}
}

// load looks a game up off the UI thread and delivers on it. stillWanted is
// asked whether the answer is still for the game on screen: the user may have
// clicked on by the time GOG answers.
func (c *metadataCache) load(gameID int, stillWanted func(int) bool, deliver func(client.GameMetadata)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		meta, err := c.fetch(ctx, gameID)
		if err != nil {
			log.Debug().Err(err).Int("gameID", gameID).Msg("No store information")
			return
		}
		runOnMain(func() {
			if stillWanted != nil && !stillWanted(gameID) {
				return
			}
			deliver(meta)
		})
	}()
}

func (c *metadataCache) pathFor(gameID int) string {
	return filepath.Join(c.dir, fmt.Sprintf("%d.json", gameID))
}
