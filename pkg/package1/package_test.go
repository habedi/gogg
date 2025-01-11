package package1

import (
	"bytes"
	"os"
	"testing"
)

// OutputCapture captures the output of a function that writes to os.Stdout.
func OutputCapture(f func()) string {
	var buf bytes.Buffer
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	// Close the writer and restore os.Stdout
	w.Close()
	os.Stdout = originalStdout

	// Read captured output
	buf.ReadFrom(r)
	r.Close()
	return buf.String()
}

// TestPrintsDoingSomething tests the output of the DoSomething function.
func TestPrintsDoingSomething(t *testing.T) {
	expected := "Doing something interesting!\n"
	output := OutputCapture(DoSomething)
	if output != expected {
		t.Errorf("Expected %q, but got %q", expected, output)
	}
}
