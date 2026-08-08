package client

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// Download errors are wrapped before they reach the reporting code, so
// cancellation has to be detected through the wrapping.
func TestIsCancellation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain cancel", context.Canceled, true},
		{"plain deadline", context.DeadlineExceeded, true},
		{"wrapped cancel", fmt.Errorf("failed to save file x: %w", context.Canceled), true},
		{"wrapped deadline", fmt.Errorf("failed to save file x: %w", context.DeadlineExceeded), true},
		{"unrelated", errors.New("HTTP 500"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCancellation(tt.err); got != tt.want {
				t.Errorf("isCancellation(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
