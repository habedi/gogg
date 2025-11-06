package cmd

import "testing"

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{999, "999 B"},
		{1024, "1.0KiB"},
		{1024*1024 + 512*1024, "1.5MiB"},
	}
	for _, c := range cases {
		got := formatBytes(c.in)
		if got != c.want {
			t.Fatalf("formatBytes(%d)=%q, want %q", c.in, got, c.want)
		}
	}
}
