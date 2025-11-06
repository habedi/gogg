package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchIdOfOwnedGames_ParsesArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"owned":[10,20,30]}`))
	}))
	defer server.Close()

	ids, err := FetchIdOfOwnedGames("token", server.URL)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ids) != 3 || ids[0] != 10 || ids[2] != 30 {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}

func TestCreateRequest_AddsAuthHeader(t *testing.T) {
	req, err := createRequest("GET", "http://example.com", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got == "" {
		t.Fatalf("missing auth header")
	}
}
