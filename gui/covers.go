package gui

import (
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
	// backgroundRendition is asked for of the picture in the game details, used
	// only when no banner is on record. GOG fades the bottom of that one to
	// white for its own pages, so what comes back has to be cropped.
	backgroundRendition = "_product_card_v2_mobile_slider_639.jpg"
)

// coverSource is where a game's artwork comes from, and whether GOG's fade is
// baked into it.
type coverSource struct {
	URL   string
	Faded bool
}

// coverSourceFor prefers the banner recorded for the game. Catalogues refreshed
// before gogg recorded banners fall back to the faded background picture.
func coverSourceFor(game db.Game) coverSource {
	if banner := strings.TrimSpace(game.CoverImage); banner != "" {
		return coverSource{URL: renditionOf(banner, bannerRendition)}
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
}

func newCoverCache(dir string) *coverCache {
	return &coverCache{
		dir:     dir,
		client:  &http.Client{Timeout: 30 * time.Second},
		fetches: make(chan struct{}, coverFetchers),
	}
}

// coverCacheDir is where covers are kept between runs.
func coverCacheDir() string {
	root := fyne.CurrentApp().Storage().RootURI()
	if root != nil && root.Path() != "" {
		return filepath.Join(root.Path(), "covers")
	}
	return filepath.Join(os.TempDir(), "gogg", "covers")
}

// fetch returns the artwork for a game, reading it from disk when it is already
// there and downloading it otherwise.
func (c *coverCache) fetch(game db.Game) ([]byte, coverSource, error) {
	source := coverSourceFor(game)
	if source.URL == "" {
		return nil, source, errors.New("game has no cover")
	}

	path := c.pathFor(source.URL)
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return data, source, nil
	}

	data, err := c.download(source.URL)
	if err != nil {
		return nil, source, err
	}

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		log.Debug().Err(err).Msg("Could not create the cover cache directory")
		return data, source, nil
	}
	// Written through a temporary file so a failed write cannot leave a
	// half-downloaded cover to be served on the next run.
	temp := path + ".part"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		log.Debug().Err(err).Msg("Could not cache cover")
		return data, source, nil
	}
	if err := os.Rename(temp, path); err != nil {
		log.Debug().Err(err).Msg("Could not cache cover")
		_ = os.Remove(temp)
	}
	return data, source, nil
}

func (c *coverCache) download(url string) ([]byte, error) {
	c.fetches <- struct{}{}
	defer func() { <-c.fetches }()

	resp, err := c.client.Get(url)
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
func (c *coverCache) load(game db.Game, stillWanted func(gameID int) bool, deliver func([]byte, coverSource)) {
	go func() {
		data, source, err := c.fetch(game)
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

func (c *coverCache) pathFor(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+".img")
}
