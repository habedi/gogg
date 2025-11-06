package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAllOwnedGameIDs_Pagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/first" {
			w.Write([]byte(`{"owned":[1,2],"next":"/second"}`))
			return
		}
		if r.URL.Path == "/second" {
			w.Write([]byte(`{"owned":[3]}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	ctx := context.Background()
	ids, err := FetchAllOwnedGameIDs(ctx, "tok", server.URL+"/first")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ids) != 3 || ids[0] != 1 || ids[2] != 3 {
		t.Fatalf("ids: %#v", ids)
	}
}
