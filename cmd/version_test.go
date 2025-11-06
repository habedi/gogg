package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCmd_PrintsInfo(t *testing.T) {
	cmd := versionCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Gogg version:") || !strings.Contains(out, "Go version:") || !strings.Contains(out, "Platform:") {
		t.Fatalf("unexpected output: %s", out)
	}
}
