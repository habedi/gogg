package cmd

import (
	"testing"

	"github.com/habedi/gogg/client"
)

func TestChangeLabel(t *testing.T) {
	tests := []struct {
		name   string
		change client.VersionChange
		want   string
	}{
		{
			name:   "added",
			change: client.VersionChange{Kind: client.ChangeAdded, NewVersion: "1.0"},
			want:   "Added",
		},
		{
			// The version fields alone cannot tell this apart from a removal.
			name:   "added without a version",
			change: client.VersionChange{Kind: client.ChangeAdded},
			want:   "Added",
		},
		{
			name:   "updated",
			change: client.VersionChange{Kind: client.ChangeUpdated, OldVersion: "1.0", NewVersion: "2.0"},
			want:   "Updated",
		},
		{
			// A game that is still owned but stops reporting a version is not
			// a removal.
			name:   "version disappeared",
			change: client.VersionChange{Kind: client.ChangeUpdated, OldVersion: "1.0"},
			want:   "Updated",
		},
		{
			name:   "removed",
			change: client.VersionChange{Kind: client.ChangeRemoved, OldVersion: "1.0"},
			want:   "Removed",
		},
		{
			name:   "removed without a version",
			change: client.VersionChange{Kind: client.ChangeRemoved},
			want:   "Removed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := changeLabel(tt.change); got != tt.want {
				t.Errorf("changeLabel(%+v) = %q, want %q", tt.change, got, tt.want)
			}
		})
	}
}
