package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAllOwnedGameIDs_LoopGuard(t *testing.T) {
	// Return next pointing to the same URL to simulate loop; we expect it to stop after first iteration
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"owned":[1],"next":"` + r.URL.String() + `"}`))
	}))
	defer server.Close()

	ctx := context.Background()
	ids, err := FetchAllOwnedGameIDs(ctx, "tok", server.URL)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("ids: %#v", ids)
	}
}
