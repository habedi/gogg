package client

import (
	"errors"
	"io"
	"net/http"
	"testing"
)

type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, errors.New("read err") }

func TestReadResponseBody_Error(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(errReader{})}
	if _, err := readResponseBody(resp); err == nil {
		t.Fatalf("expected error")
	}
}
