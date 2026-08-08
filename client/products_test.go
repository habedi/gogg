package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// The artwork in the game details has GOG's fade baked in; the owned-products
// listing points at the unfaded banner instead. The listing is asked for
// sorted by purchase date, so a product's position is its purchase rank.
func TestFetchOwnedProducts_ReadsEveryPageInPurchaseOrder(t *testing.T) {
	var requested atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested.Add(1)
		require.Equal(t, "date_purchased", r.URL.Query().Get("sortBy"),
			"the listing has to be asked for in purchase order")
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = w.Write([]byte(`{"totalPages":2,"products":[
				{"id":1,"image":"//images-1.gog-statics.com/aaa"},
				{"id":2,"image":"//images-2.gog-statics.com/bbb"}]}`))
		default:
			_, _ = w.Write([]byte(`{"totalPages":2,"products":[{"id":3,"image":"//images-3.gog-statics.com/ccc"}]}`))
		}
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	products, err := FetchOwnedProducts(context.Background(), "tok")
	require.NoError(t, err)

	require.Equal(t, map[int]OwnedProduct{
		1: {Image: "//images-1.gog-statics.com/aaa", PurchaseRank: 1},
		2: {Image: "//images-2.gog-statics.com/bbb", PurchaseRank: 2},
		3: {Image: "//images-3.gog-statics.com/ccc", PurchaseRank: 3},
	}, products)
	require.Equal(t, int64(2), requested.Load())
}

// A product without artwork still has a place in the purchase order.
func TestFetchOwnedProducts_KeepsProductsWithoutArtwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"totalPages":1,"products":[{"id":1,"image":""},{"id":2,"image":"//x/y"}]}`))
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	products, err := FetchOwnedProducts(context.Background(), "tok")
	require.NoError(t, err)
	require.Equal(t, map[int]OwnedProduct{
		1: {Image: "", PurchaseRank: 1},
		2: {Image: "//x/y", PurchaseRank: 2},
	}, products)
}

func TestFetchOwnedProducts_ReportsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	_, err := FetchOwnedProducts(context.Background(), "tok")
	require.Error(t, err)
}

// A server claiming an absurd number of pages must not keep gogg going forever.
func TestFetchOwnedProducts_StopsAtAPageLimit(t *testing.T) {
	var requested atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requested.Add(1)
		_, _ = fmt.Fprint(w, `{"totalPages":100000,"products":[]}`)
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	_, err := FetchOwnedProducts(context.Background(), "tok")
	require.NoError(t, err)
	require.LessOrEqual(t, requested.Load(), int64(maxProductPages))
}
