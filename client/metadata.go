package client

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// apiBase is where the store API lives. Overridable for tests.
func apiBase() string {
	if v := strings.TrimSpace(os.Getenv("GOGG_API_BASE")); v != "" {
		return v
	}
	return "https://api.gog.com"
}

// Screenshot is one picture from a game's store page, at the two sizes gogg
// shows it: a thumbnail in the strip and a larger one when it is opened.
type Screenshot struct {
	ThumbnailURL string
	LargeURL     string
}

const (
	// screenshotThumbFormatter selects a 112x63 rendition, about 2 KB.
	screenshotThumbFormatter = "product_card_screenshot_112"
	// screenshotLargeFormatter selects a 748x421 rendition, about 90 KB. GOG
	// also offers a 1600 one, which is a wider crop and too big to be worth it.
	screenshotLargeFormatter = "product_card_screenshot_748"
)

// GameMetadata is what GOG's store publishes about a game, beyond the files it
// offers. Every field is optional: games delisted from the store keep working
// in a library but stop being described.
type GameMetadata struct {
	Summary     string
	Developers  []string
	Publisher   string
	Genres      []string
	Features    []string
	AgeRating   string
	ReleaseDate string
	InstalledMB int64
	StoreURL    string
	ForumURL    string
	BoxArtURL   string
	Screenshots []Screenshot
}

// FetchGameMetadata returns the store information GOG publishes for a game.
// Both endpoints are public, so this needs no token.
func FetchGameMetadata(ctx context.Context, productID int) (GameMetadata, error) {
	var game v2Game
	if err := getJSON(ctx, fmt.Sprintf("%s/v2/games/%d", apiBase(), productID), &game); err != nil {
		return GameMetadata{}, fmt.Errorf("failed to fetch game metadata: %w", err)
	}

	meta := GameMetadata{
		Summary:     plainText(game.Overview),
		Publisher:   game.Embedded.Publisher.Name,
		AgeRating:   game.Embedded.ESRBRating.Category.Name,
		InstalledMB: game.Size,
		StoreURL:    game.Links.Store.Href,
		ForumURL:    game.Links.Forum.Href,
		BoxArtURL:   game.Links.BoxArtImage.Href,
	}
	for _, developer := range game.Embedded.Developers {
		meta.Developers = append(meta.Developers, developer.Name)
	}
	for _, tag := range game.Embedded.Tags {
		meta.Genres = append(meta.Genres, tag.Name)
	}
	for _, feature := range game.Embedded.Features {
		meta.Features = append(meta.Features, feature.Name)
	}
	for _, shot := range game.Embedded.Screenshots {
		address := strings.TrimSpace(shot.Links.Self.Href)
		if address == "" {
			continue
		}
		meta.Screenshots = append(meta.Screenshots, Screenshot{
			ThumbnailURL: screenshotRendition(address, screenshotThumbFormatter),
			LargeURL:     screenshotRendition(address, screenshotLargeFormatter),
		})
	}

	// The release date lives on the older endpoint. It is the only thing that
	// does, so losing it must not lose everything else.
	var product productInfo
	if err := getJSON(ctx, fmt.Sprintf("%s/products/%d", apiBase(), productID), &product); err != nil {
		log.Debug().Err(err).Int("gameID", productID).Msg("No product information")
		return meta, nil
	}
	meta.ReleaseDate = releaseDate(product.ReleaseDate)
	if meta.StoreURL == "" {
		meta.StoreURL = product.Links.ProductCard
	}
	return meta, nil
}

// v2Game is the part of GOG's game endpoint gogg reads.
type v2Game struct {
	Overview string `json:"overview"`
	Size     int64  `json:"size"` // installed size in megabytes
	Links    struct {
		Store       struct{ Href string } `json:"store"`
		Forum       struct{ Href string } `json:"forum"`
		BoxArtImage struct{ Href string } `json:"boxArtImage"`
	} `json:"_links"`
	Embedded struct {
		Developers []struct{ Name string } `json:"developers"`
		Publisher  struct{ Name string }   `json:"publisher"`
		Tags       []struct{ Name string } `json:"tags"`
		Features   []struct{ Name string } `json:"features"`
		ESRBRating struct {
			Category struct{ Name string } `json:"category"`
		} `json:"esrbRating"`
		Screenshots []struct {
			Links struct {
				Self struct {
					Href string `json:"href"`
				} `json:"self"`
			} `json:"_links"`
		} `json:"screenshots"`
	} `json:"_embedded"`
}

// productInfo is the part of GOG's product endpoint gogg still needs.
type productInfo struct {
	ReleaseDate string `json:"release_date"`
	Links       struct {
		ProductCard string `json:"product_card"`
	} `json:"links"`
}

func getJSON(ctx context.Context, url string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, into)
}

// screenshotRendition fills in the size GOG leaves as a placeholder in the
// address it publishes.
func screenshotRendition(address, formatter string) string {
	return strings.ReplaceAll(address, "{formatter}", formatter)
}

// releaseDate trims GOG's timestamp to the day, which is all that is worth
// showing.
func releaseDate(timestamp string) string {
	timestamp = strings.TrimSpace(timestamp)
	if timestamp == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
		return parsed.Format("2006-01-02")
	}
	if len(timestamp) >= 10 {
		return timestamp[:10]
	}
	return timestamp
}

var (
	htmlBreaks = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>`)
	htmlTags   = regexp.MustCompile(`<[^>]*>`)
	blankLines = regexp.MustCompile(`\n{2,}`)
)

// plainText turns the HTML GOG writes its descriptions in into something a
// label can show.
func plainText(markup string) string {
	text := htmlBreaks.ReplaceAllString(markup, "\n")
	text = htmlTags.ReplaceAllString(text, "")
	text = html.UnescapeString(text)

	lines := strings.Split(text, "\n")
	kept := lines[:0]
	for _, line := range lines {
		kept = append(kept, strings.TrimSpace(line))
	}
	text = strings.Join(kept, "\n")
	text = blankLines.ReplaceAllString(text, "\n")
	return strings.TrimSpace(text)
}
