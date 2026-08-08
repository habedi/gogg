package client

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestFetchAllOwnedGameIDs_CycleDetection(t *testing.T) {
	// Server always returns the same next URL, creating an infinite loop that
	// the cycle-detection guard must break.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// next resolves back to the same URL → cycle
		w.Write([]byte(`{"owned":[1],"next":"/games"}`))
	}))
	defer server.Close()

	ids, err := FetchAllOwnedGameIDs(context.Background(), "tok", server.URL+"/games")
	require.NoError(t, err)
	assert.Equal(t, []int{1}, ids)
}

func TestFetchAllOwnedGameIDs_BadJSONBody(t *testing.T) {
	// Server returns invalid JSON: the page cannot be trusted, so the error is
	// reported rather than treated as the end of the listing.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	_, err := FetchAllOwnedGameIDs(context.Background(), "tok", server.URL)
	assert.Error(t, err)
}

func TestFetchAllOwnedGameIDs_SendRequestError(t *testing.T) {
	// Server returns 401 → sendRequest returns a non-2xx error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := FetchAllOwnedGameIDs(context.Background(), "tok", server.URL)
	assert.Error(t, err)
}

func TestFetchAllOwnedGameIDs_ReadBodyError(t *testing.T) {
	// Server sends 200 headers then drops the connection, causing readResponseBody
	// to fail. The error must reach the caller.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("httptest server does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\nContent-Type: application/json\r\n\r\n"))
		conn.(*net.TCPConn).CloseWrite()
		conn.Close()
	}))
	defer server.Close()

	_, err := FetchAllOwnedGameIDs(context.Background(), "tok", server.URL)
	assert.Error(t, err)
}
