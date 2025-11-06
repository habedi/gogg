package client

import "testing"

func TestSanitizePath_EdgeCases(t *testing.T) {
	cases := []struct{ in, wantPrefix string }{
		{"", ""},
		{"!!!***???", ""},
		{"This Is A Very Long Name With Spaces And ™® ??? ", "this-is-a-very-long-name"},
		{"/path/to\\file:name|*?\"'", "path-to-file"},
	}
	for _, c := range cases {
		out := SanitizePath(c.in)
		if c.wantPrefix == "" && out != "" {
			t.Fatalf("expected empty, got %q for %q", out, c.in)
		}
		if c.wantPrefix != "" && (len(out) == 0 || out[:len(c.wantPrefix)] != c.wantPrefix) {
			t.Fatalf("expected prefix %q, got %q for %q", c.wantPrefix, out, c.in)
		}
	}
}

func TestURLHelpers(t *testing.T) {
	if !isAbsoluteURL("https://example.com/file") {
		t.Fatal("expected absolute")
	}
	if isAbsoluteURL("/relative/path") {
		t.Fatal("expected relative")
	}

	abs := buildManualURL("https://e/x")
	if abs != "https://e/x" {
		t.Fatalf("abs passthrough: %s", abs)
	}
	rel := buildManualURL("/file")
	if rel == "/file" || rel == "" {
		t.Fatalf("rel should be prefixed, got %q", rel)
	}
}

func TestResolveNext(t *testing.T) {
	base := "https://example.com/user/data/games"
	if got := resolveNext(base, ""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := resolveNext(base, "https://a/b"); got != "https://a/b" {
		t.Fatalf("abs: %s", got)
	}
	if got := resolveNext(base, "/next?p=2"); got != "https://example.com/next?p=2" {
		t.Fatalf("rel: %s", got)
	}
}
