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
// listing points at the unfaded banner instead.
func TestFetchOwnedProductImages_ReadsEveryPage(t *testing.T) {
	var requested atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested.Add(1)
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

	images, err := FetchOwnedProductImages(context.Background(), "tok")
	require.NoError(t, err)

	require.Equal(t, map[int]string{
		1: "//images-1.gog-statics.com/aaa",
		2: "//images-2.gog-statics.com/bbb",
		3: "//images-3.gog-statics.com/ccc",
	}, images)
	require.Equal(t, int64(2), requested.Load())
}

func TestFetchOwnedProductImages_SkipsProductsWithoutArtwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"totalPages":1,"products":[{"id":1,"image":""},{"id":2,"image":"//x/y"}]}`))
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	images, err := FetchOwnedProductImages(context.Background(), "tok")
	require.NoError(t, err)
	require.Equal(t, map[int]string{2: "//x/y"}, images)
}

func TestFetchOwnedProductImages_ReportsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	_, err := FetchOwnedProductImages(context.Background(), "tok")
	require.Error(t, err)
}

// A server claiming an absurd number of pages must not keep gogg going forever.
func TestFetchOwnedProductImages_StopsAtAPageLimit(t *testing.T) {
	var requested atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requested.Add(1)
		_, _ = fmt.Fprint(w, `{"totalPages":100000,"products":[]}`)
	}))
	defer srv.Close()
	t.Setenv("GOGG_EMBED_BASE", srv.URL)

	_, err := FetchOwnedProductImages(context.Background(), "tok")
	require.NoError(t, err)
	require.LessOrEqual(t, requested.Load(), int64(maxProductPages))
}
