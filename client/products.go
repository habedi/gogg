package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
)

// maxProductPages stops a server that reports an absurd page count from keeping
// gogg fetching forever. At 50 products a page this covers 5000 games.
const maxProductPages = 100

// productsPage is the part of the owned-products listing gogg reads.
type productsPage struct {
	TotalPages int `json:"totalPages"`
	Products   []struct {
		ID    int    `json:"id"`
		Image string `json:"image"`
	} `json:"products"`
}

// FetchOwnedProductImages returns the artwork GOG lists for each owned game,
// keyed by product ID. This is not the same picture as the one in the game
// details: that one has a fade to white baked in for GOG's own pages, while
// this is the plain banner.
func FetchOwnedProductImages(ctx context.Context, accessToken string) (map[int]string, error) {
	images := make(map[int]string)

	for page, totalPages := 1, 1; page <= totalPages && page <= maxProductPages; page++ {
		url := fmt.Sprintf("%s/account/getFilteredProducts?mediaType=1&page=%d", embedBase(), page)
		req, err := createRequest(ctx, "GET", url, accessToken)
		if err != nil {
			return nil, err
		}

		resp, err := sendRequest(req)
		if err != nil {
			return nil, err
		}

		var listing productsPage
		if err := func() error {
			defer func() { _ = resp.Body.Close() }()
			body, err := readResponseBody(resp)
			if err != nil {
				return fmt.Errorf("failed to read owned products page %d: %w", page, err)
			}
			if err := json.Unmarshal(body, &listing); err != nil {
				return fmt.Errorf("failed to parse owned products page %d: %w", page, err)
			}
			return nil
		}(); err != nil {
			return nil, err
		}

		if listing.TotalPages > 0 {
			totalPages = listing.TotalPages
		}
		for _, product := range listing.Products {
			if product.Image != "" {
				images[product.ID] = product.Image
			}
		}
	}

	log.Debug().Int("count", len(images)).Msg("Fetched game artwork references")
	return images, nil
}
