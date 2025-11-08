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
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := &auth.Service{Storer: testStorer{}}
	executeDownload(ctx, svc, 1, "/tmp", "en", "windows", false, false, true, true, false, false, 1)
}
