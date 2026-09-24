package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/clierr"
)

type testStorer struct{}

func (testStorer) GetTokenRecord() (*db.Token, error)      { return nil, nil }
func (testStorer) UpsertTokenRecord(token *db.Token) error { return nil }

func TestCliErrTypes(t *testing.T) {
	// Validation example
	e := clierr.New(clierr.Validation, "Invalid platform", errors.New("bad"))
	if e.Type != clierr.Validation {
		t.Fatalf("type mismatch: %v", e.Type)
	}
	if e.Error() == "" {
		t.Fatal("empty message")
	}
	if !errors.Is(e, e.Err) {
		t.Fatal("unwrap failed")
	}
}

// Simple cancellation smoke test: executeDownload should handle cancelled context gracefully.
func TestExecuteDownload_Cancel(t *testing.T) {
	setLastCliErr(nil)
	t.Cleanup(func() { setLastCliErr(nil) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := &auth.Service{Storer: testStorer{}}
	executeDownload(ctx, svc, 1, "/tmp", "en", "windows", false, false, true, true, false, false, false, false, false, 1, 1)

	err := getLastCliErr()
	if err == nil {
		t.Fatal("expected lastCliErr to be set on cancelled download")
	}
	if err.Type != clierr.Internal {
		t.Fatalf("expected clierr.Internal, got %v", err.Type)
	}
}

func TestExecuteDownload_ValidationErrors(t *testing.T) {
	svc := &auth.Service{Storer: testStorer{}}
	ctx := context.Background()

	tests := []struct {
		name        string
		threads     int
		connections int
		platform    string
		language    string
	}{
		{name: "invalid threads", threads: 99, connections: 1, platform: "windows", language: "en"},
		{name: "invalid connections", threads: 2, connections: 99, platform: "windows", language: "en"},
		{name: "invalid platform", threads: 2, connections: 1, platform: "atari", language: "en"},
		{name: "invalid language", threads: 2, connections: 1, platform: "windows", language: "klingon"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setLastCliErr(nil)
			t.Cleanup(func() { setLastCliErr(nil) })

			executeDownload(ctx, svc, 1, "/tmp", tc.language, tc.platform, false, false, true, true, false, false, false, false, false, tc.threads, tc.connections)

			err := getLastCliErr()
			if err == nil {
				t.Fatalf("%s: expected lastCliErr to be set", tc.name)
			}
			if err.Type != clierr.Validation {
				t.Fatalf("%s: expected clierr.Validation, got %v", tc.name, err.Type)
			}
		})
	}
}
