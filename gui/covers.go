package gui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
)

// maxCoverBytes caps what a single cover may cost us in memory and disk.
const maxCoverBytes = 8 << 20

// coverFetchers bounds how many covers are fetched at once, so scrolling the
// library does not open a connection per game.
const coverFetchers = 4

const (
	// bannerRendition is asked for of the artwork GOG lists for owned games:
	// 392x220 with nothing baked in.
	bannerRendition = "_392.jpg"
	// thumbnailRendition is the same artwork at 100x60, about 1.5 KB, which is
	// all a list row needs.
	thumbnailRendition = "_prof_game_100x60.jpg"
	// backgroundRendition is asked for of the picture in the game details, used
	// only when no banner is on record. GOG fades the bottom of that one to
	// white for its own pages, so what comes back has to be cropped.
	backgroundRendition = "_product_card_v2_mobile_slider_639.jpg"
)

// coverKind is how large a picture the caller needs.
type coverKind int

const (
	// coverBanner suits the grid and the details pane.
	coverBanner coverKind = iota
	// coverThumbnail suits a list row.
	coverThumbnail
)

// coverSource is where a game's artwork comes from, and whether GOG's fade is
// baked into it.
type coverSource struct {
	URL   string
	Faded bool
}

// coverSourceFor prefers the banner recorded for the game. Catalogues refreshed
// before gogg recorded banners fall back to the faded background picture.
func coverSourceFor(game db.Game, kind coverKind) coverSource {
	rendition := bannerRendition
	if kind == coverThumbnail {
		rendition = thumbnailRendition
	}

	if banner := strings.TrimSpace(game.CoverImage); banner != "" {
		return coverSource{URL: renditionOf(banner, rendition)}
	}

	parsed, err := client.ParseGameData(game.Data)
	if err != nil || parsed.BackgroundImage == nil {
		return coverSource{}
	}
	background := strings.TrimSpace(*parsed.BackgroundImage)
	if background == "" {
		return coverSource{}
	}
	return coverSource{URL: renditionOf(background, backgroundRendition), Faded: true}
}

// renditionOf completes a GOG image address. They are protocol-relative, and
// the bare hash answers 404: a suffix chooses which rendition to serve.
func renditionOf(address, rendition string) string {
	if strings.HasPrefix(address, "//") {
		address = "https:" + address
	}
	if path.Ext(address) == "" {
		address += rendition
	}
	return address
}

// coverCache fetches game artwork once and keeps it on disk between runs.
type coverCache struct {
	dir     string
	client  *http.Client
	fetches chan struct{}
	// ctx ends the fetches when the cache closes, and wg is how close waits
	// for the ones already in flight, deliveries included. A delivery that
	// outlives its owner lands in widgets someone else is using by then.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// close stops new fetches and waits out the in-flight ones. After it
// returns, no delivery of this cache will touch a widget again.
func (c *coverCache) close() {
	c.cancel()
	c.wg.Wait()
}

func newCoverCache(dir string) *coverCache {
	ctx, cancel := context.WithCancel(context.Background())
	return &coverCache{
		dir:     dir,
		client:  &http.Client{Timeout: 30 * time.Second},
		fetches: make(chan struct{}, coverFetchers),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// coverCacheDir is where covers are kept between runs.
func coverCacheDir() string {
	return filepath.Join(cacheRoot(), "covers")
}

// cacheRoot is where gogg keeps what it has downloaded about a game rather than
// from it. Tests replace it so they never share a directory with a real run.
var cacheRoot = func() string {
	root := fyne.CurrentApp().Storage().RootURI()
	if root != nil && root.Path() != "" {
		return root.Path()
	}
	return filepath.Join(os.TempDir(), "gogg")
}

// fetch returns the artwork for a game, reading it from disk when it is already
// there and downloading it otherwise.
func (c *coverCache) fetch(game db.Game, kind coverKind) ([]byte, coverSource, error) {
	source := coverSourceFor(game, kind)
	if source.URL == "" {
		return nil, source, errors.New("game has no cover")
	}

	data, err := c.fetchURL(source.URL)
	if err != nil {
		return nil, source, err
	}
	return data, source, nil
}

// fetchURL returns one picture, from disk when it is already there and by
// downloading it otherwise. Screenshots come through here too, so they share
// the cache directory and the limit on connections.
func (c *coverCache) fetchURL(url string) ([]byte, error) {
	path := c.pathFor(url)
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return data, nil
	}

	data, err := c.download(url)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		log.Debug().Err(err).Msg("Could not create the cover cache directory")
		return data, nil
	}
	// Written through a temporary file so a failed write cannot leave a
	// half-downloaded picture to be served on the next run.
	temp := path + ".part"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		log.Debug().Err(err).Msg("Could not cache cover")
		return data, nil
	}
	if err := os.Rename(temp, path); err != nil {
		log.Debug().Err(err).Msg("Could not cache cover")
		_ = os.Remove(temp)
	}
	return data, nil
}

func (c *coverCache) download(url string) ([]byte, error) {
	c.fetches <- struct{}{}
	defer func() { <-c.fetches }()

	req, err := http.NewRequestWithContext(c.ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cover: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cover: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch cover: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read cover: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("cover was empty")
	}
	return data, nil
}

// load fetches a cover off the UI thread and hands it back on the UI thread.
// stillWanted is asked whether the answer is still for the game the caller is
// showing: grid cells are recycled while a fetch is in flight.
func (c *coverCache) load(game db.Game, kind coverKind, stillWanted func(gameID int) bool, deliver func([]byte, coverSource)) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		data, source, err := c.fetch(game, kind)
		if err != nil {
			log.Debug().Err(err).Str("game", game.Title).Msg("No cover")
			return
		}
		runOnMain(func() {
			if stillWanted != nil && !stillWanted(game.ID) {
				return
			}
			deliver(data, source)
		})
	}()
}

// loadURL fetches one picture off the UI thread and hands it back on the UI
// thread. stillWanted is asked whether the answer is still worth showing: the
// user may have selected another game while it was in flight.
func (c *coverCache) loadURL(url string, stillWanted func() bool, deliver func([]byte)) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		data, err := c.fetchURL(url)
		if err != nil {
			log.Debug().Err(err).Str("url", url).Msg("Could not fetch picture")
			return
		}
		runOnMain(func() {
			if stillWanted != nil && !stillWanted() {
				return
			}
			deliver(data)
		})
	}()
}

func (c *coverCache) pathFor(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+".img")
}
