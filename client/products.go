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

// OwnedProduct is what the owned-products listing says about one game beyond
// its files: its artwork, and its place in the by-purchase-date order.
type OwnedProduct struct {
	// Image is the plain banner GOG lists for the game. This is not the same
	// picture as the one in the game details: that one has a fade to white
	// baked in for GOG's own pages.
	Image string
	// PurchaseRank is the game's position in the listing sorted by purchase
	// date, 1 being the most recent buy. GOG publishes the order but not the
	// dates themselves.
	PurchaseRank int
}

// FetchOwnedProducts returns what GOG's account listing says about each owned
// game, keyed by product ID.
func FetchOwnedProducts(ctx context.Context, accessToken string) (map[int]OwnedProduct, error) {
	products := make(map[int]OwnedProduct)

	rank := 0
	for page, totalPages := 1, 1; page <= totalPages && page <= maxProductPages; page++ {
		// Sorted by purchase date, so the position of a product is its rank.
		url := fmt.Sprintf("%s/account/getFilteredProducts?mediaType=1&sortBy=date_purchased&page=%d",
			embedBase(), page)
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
			rank++
			products[product.ID] = OwnedProduct{Image: product.Image, PurchaseRank: rank}
		}
	}

	log.Debug().Int("count", len(products)).Msg("Fetched owned product listing")
	return products, nil
}
