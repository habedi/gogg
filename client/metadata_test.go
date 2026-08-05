package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

const v2Response = `{
	"overview": "\n<b>Enter the Norse realm</b>\n<br>\nHis vengeance against the Gods &amp; monsters.\n",
	"size": 40981,
	"_links": {
		"store": {"href": "https://www.gog.com/en/game/god_of_war"},
		"forum": {"href": "https://www.gog.com/forum/god_of_war"},
		"boxArtImage": {"href": "https://images.gog-statics.com/boxart.jpg"}
	},
	"_embedded": {
		"developers": [{"name": "Santa Monica Studio"}],
		"publisher": {"name": "PlayStation PC LLC"},
		"tags": [{"name": "Action"}, {"name": "Adventure"}],
		"features": [{"id": "achievements", "name": "Achievements"}, {"id": "single", "name": "Single-player"}],
		"esrbRating": {"category": {"name": "Mature 17+"}},
		"screenshots": [
			{"_links": {"self": {
				"href": "https://images.gog-statics.com/aaa_{formatter}.jpg",
				"templated": true,
				"formatters": ["product_card_screenshot_112", "product_card_screenshot_748", "1600"]
			}}},
			{"_links": {"self": {"href": "https://images.gog-statics.com/plain.jpg"}}},
			{"_links": {"self": {"href": ""}}}
		]
	}
}`

const productResponse = `{
	"release_date": "2024-03-12T14:50:00+0100",
	"links": {"product_card": "https://www.gog.com/game/god_of_war"}
}`

func metadataServer(t *testing.T, v2Status, productStatus int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/games/42":
			if v2Status != http.StatusOK {
				w.WriteHeader(v2Status)
				return
			}
			_, _ = w.Write([]byte(v2Response))
		case "/products/42":
			if productStatus != http.StatusOK {
				w.WriteHeader(productStatus)
				return
			}
			_, _ = w.Write([]byte(productResponse))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The store information lives across two endpoints, and neither needs a token.
func TestFetchGameMetadata_MergesBothEndpoints(t *testing.T) {
	srv := metadataServer(t, http.StatusOK, http.StatusOK)
	t.Setenv("GOGG_API_BASE", srv.URL)

	meta, err := FetchGameMetadata(context.Background(), 42)
	require.NoError(t, err)

	require.Contains(t, meta.Summary, "Enter the Norse realm")
	require.NotContains(t, meta.Summary, "<b>", "the summary is HTML and has to be readable as text")
	require.Contains(t, meta.Summary, "Gods & monsters", "entities are decoded")

	require.Equal(t, []string{"Santa Monica Studio"}, meta.Developers)
	require.Equal(t, "PlayStation PC LLC", meta.Publisher)
	require.Equal(t, []string{"Action", "Adventure"}, meta.Genres)
	require.Equal(t, []string{"Achievements", "Single-player"}, meta.Features)
	require.Equal(t, "Mature 17+", meta.AgeRating)
	require.Equal(t, int64(40981), meta.InstalledMB)
	require.Equal(t, "https://www.gog.com/en/game/god_of_war", meta.StoreURL)
	require.Equal(t, "https://images.gog-statics.com/boxart.jpg", meta.BoxArtURL)
	require.Equal(t, "2024-03-12", meta.ReleaseDate, "the timestamp is trimmed to the date")

	// A strip of thumbnails costs 2 KB a picture; the one that is opened costs
	// 90 KB, so the two renditions are kept apart.
	require.Len(t, meta.Screenshots, 2, "a screenshot with no address is dropped")
	require.Equal(t, Screenshot{
		ThumbnailURL: "https://images.gog-statics.com/aaa_product_card_screenshot_112.jpg",
		LargeURL:     "https://images.gog-statics.com/aaa_product_card_screenshot_748.jpg",
	}, meta.Screenshots[0])
	require.Equal(t, Screenshot{
		ThumbnailURL: "https://images.gog-statics.com/plain.jpg",
		LargeURL:     "https://images.gog-statics.com/plain.jpg",
	}, meta.Screenshots[1], "an address with no rendition to choose is used as it is")
}

// The second endpoint only adds the release date, so losing it must not lose
// everything else.
func TestFetchGameMetadata_SurvivesTheProductEndpointFailing(t *testing.T) {
	srv := metadataServer(t, http.StatusOK, http.StatusInternalServerError)
	t.Setenv("GOGG_API_BASE", srv.URL)

	meta, err := FetchGameMetadata(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "PlayStation PC LLC", meta.Publisher)
	require.Empty(t, meta.ReleaseDate)
}

// Delisted games are gone from the store even though they stay in the library.
func TestFetchGameMetadata_ReportsAMissingGame(t *testing.T) {
	srv := metadataServer(t, http.StatusNotFound, http.StatusNotFound)
	t.Setenv("GOGG_API_BASE", srv.URL)

	_, err := FetchGameMetadata(context.Background(), 42)
	require.Error(t, err)
}

func TestPlainText(t *testing.T) {
	require.Equal(t, "First line\nSecond line",
		plainText("<p>First line</p><p>Second line</p>"))
	require.Equal(t, "Bold and <not a tag",
		plainText("<b>Bold</b> and &lt;not a tag"))
	require.Equal(t, "One\nTwo", plainText("One<br/>Two"))
	require.Empty(t, plainText("   <div>  </div>  "))
}
