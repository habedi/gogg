package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateRequest(t *testing.T) {
	req, err := createRequest("GET", "http://example.com/path", "mytoken")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	auth := req.Header.Get("Authorization")
	expected := "Bearer mytoken"
	if auth != expected {
		t.Errorf("Expected Authorization header %q, got %q", expected, auth)
	}
}

func TestParseRawDownloads(t *testing.T) {
	raw := [][]interface{}{
		{"en", map[string]interface{}{"windows": []interface{}{map[string]interface{}{"name": "setup.exe", "size": "1GB"}}}},
		{123, "invalid"},
		{"fr"},
	}
	downloads := parseRawDownloads(raw)
	if len(downloads) != 1 {
		t.Fatalf("Expected 1 valid download entry, got %d", len(downloads))
	}
	dl := downloads[0]
	assert.Equal(t, "en", dl.Language)
	assert.Len(t, dl.Platforms.Windows, 1)
	file := dl.Platforms.Windows[0]
	assert.Equal(t, "setup.exe", file.Name)
	assert.Equal(t, "1GB", file.Size)
}
