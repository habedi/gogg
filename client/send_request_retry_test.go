package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSendRequest_RetriesOn500ThenSucceeds(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("boom"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	delays := stubRetryWait(t, nil)
	resp, err := sendRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if got := delays(); len(got) != 1 || got[0] != time.Second {
		// The retry has to back off once before the second attempt.
		t.Fatalf("expected a single 1s backoff, got %v", got)
	}
	_ = resp.Body.Close()
}
