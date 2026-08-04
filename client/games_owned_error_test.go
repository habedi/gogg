package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A broken page in the middle of the owned-games listing must be reported.
// Returning a short list as if it were complete makes the caller believe the
// user no longer owns the missing games.
func TestFetchAllOwnedGameIDs_ErrorOnUnparseablePage(t *testing.T) {
	var hits atomic.Int64
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"owned":[1,2,3],"next":"` + srv.URL + `/page2"}`))
			return
		}
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer srv.Close()

	ids, err := FetchAllOwnedGameIDs(context.Background(), "tok", srv.URL+"/page1")
	if err == nil {
		t.Fatalf("expected an error, got a truncated list: %v", ids)
	}
}
