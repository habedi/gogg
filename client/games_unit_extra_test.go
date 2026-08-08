package client

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchIdOfOwnedGames_ParsesArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"owned":[10,20,30]}`))
	}))
	defer server.Close()

	ids, err := FetchIdOfOwnedGames(context.Background(), "token", server.URL)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ids) != 3 || ids[0] != 10 || ids[2] != 30 {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}

func TestCreateRequest_AddsAuthHeader(t *testing.T) {
	req, err := createRequest(context.Background(), "GET", "http://example.com", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got == "" {
		t.Fatalf("missing auth header")
	}
}

func TestCreateRequest_InvalidURL(t *testing.T) {
	// A null byte is an invalid control character that url.Parse rejects.
	_, err := createRequest(context.Background(), "GET", "\x00", "token")
	assert.Error(t, err)
}

func TestSendRequest_NonTwoXXReturnsError(t *testing.T) {
	// 404 breaks the retry loop immediately (only 5xx triggers retry+sleep).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	req, err := createRequest(context.Background(), "GET", server.URL, "token")
	require.NoError(t, err)
	_, err = sendRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestCloseResponseBody_NilResponse(t *testing.T) {
	closeResponseBody(nil) // must not panic
}

func TestCloseResponseBody_NilBody(t *testing.T) {
	closeResponseBody(&http.Response{}) // Body is nil and must not panic
}

func TestParseGameData_InvalidJSON(t *testing.T) {
	var g Game
	err := parseGameData([]byte("not json"), &g)
	assert.Error(t, err)
}

func TestUnmarshalJSON_ArrayInput(t *testing.T) {
	// A JSON array is syntactically valid so UnmarshalJSON is invoked, but the
	// inner json.Unmarshal into the anonymous struct fails because the value is
	// not a JSON object. This covers the inner-unmarshal error branch.
	var g Game
	err := json.Unmarshal([]byte("[]"), &g)
	assert.Error(t, err)
}

func TestParseOwnedGames_InvalidJSON(t *testing.T) {
	_, err := parseOwnedGames([]byte("not json"))
	assert.Error(t, err)
}

func TestParseSizeString_UnknownUnit(t *testing.T) {
	_, err := parseSizeString("5 xyz")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown size unit")
}

func TestParseSizeString_BareIntegerNoUnit(t *testing.T) {
	got, err := parseSizeString("1024")
	require.NoError(t, err)
	assert.Equal(t, int64(1024), got)
}

func TestParseSizeString_Empty(t *testing.T) {
	_, err := parseSizeString("")
	assert.Error(t, err)
}

func TestParseSizeString_NegativeInteger(t *testing.T) {
	// "-1024" does not match the regex (no leading digit) so it falls through
	// to the ParseInt fallback, covering the return-value path there.
	got, err := parseSizeString("-1024")
	require.NoError(t, err)
	assert.Equal(t, int64(-1024), got)
}

func TestFetchGameData_InvalidJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json at all"))
	}))
	defer server.Close()

	_, _, err := FetchGameData(context.Background(), "token", server.URL)
	assert.Error(t, err)
}

func TestFetchIdOfOwnedGames_InvalidJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json at all"))
	}))
	defer server.Close()

	_, err := FetchIdOfOwnedGames(context.Background(), "token", server.URL)
	assert.Error(t, err)
}

func TestFetchGameData_InvalidURL(t *testing.T) {
	// Covers the createRequest error branch inside FetchGameData.
	_, _, err := FetchGameData(context.Background(), "token", "\x00")
	assert.Error(t, err)
}

func TestFetchIdOfOwnedGames_InvalidURL(t *testing.T) {
	// Covers the createRequest error branch inside FetchIdOfOwnedGames.
	_, err := FetchIdOfOwnedGames(context.Background(), "token", "\x00")
	assert.Error(t, err)
}

// droppedBodyServer returns a test server that sends HTTP 200 headers with
// Content-Length: 100 and then immediately closes the connection, causing
// io.ReadAll to return an error when reading the truncated body.
func droppedBodyServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		// Send a well-formed 200 header but drop the body.
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\nContent-Type: application/json\r\n\r\n"))
		conn.(*net.TCPConn).CloseWrite()
		conn.Close()
	}))
}

func TestFetchIdOfOwnedGames_ReadBodyError(t *testing.T) {
	server := droppedBodyServer(t)
	defer server.Close()

	_, err := FetchIdOfOwnedGames(context.Background(), "token", server.URL)
	assert.Error(t, err)
}

func TestFetchGameData_ReadBodyError(t *testing.T) {
	server := droppedBodyServer(t)
	defer server.Close()

	_, _, err := FetchGameData(context.Background(), "token", server.URL)
	assert.Error(t, err)
}
