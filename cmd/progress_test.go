package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilesWord(t *testing.T) {
	require.Equal(t, "file", filesWord(1))
	require.Equal(t, "files", filesWord(0))
	require.Equal(t, "files", filesWord(2))
}

func TestValidateCredentials(t *testing.T) {
	require.True(t, validateCredentials("user", "pass"))
	require.False(t, validateCredentials("", "pass"))
	require.False(t, validateCredentials("user", ""))
	require.False(t, validateCredentials("", ""))
}

func TestGetFileStatusString(t *testing.T) {
	cw := &cliProgressWriter{fileProgress: map[string]struct{ current, total int64 }{}}
	require.Equal(t, "Finalizing...", cw.getFileStatusString(), "nothing in flight reads as finalizing")

	cw.fileProgress["setup.exe"] = struct{ current, total int64 }{5, 10}
	s := cw.getFileStatusString()
	require.Contains(t, s, "1 file")
	require.Contains(t, s, "setup.exe")
	require.Contains(t, s, "50%")
}

// The CLI progress writer parses the JSON-line stream the client emits and
// accumulates the total across files.
func TestCliProgressWriter_ParsesTheStream(t *testing.T) {
	cw := &cliProgressWriter{}
	stream := `{"type":"start","overall_total":100}` + "\n" +
		`{"type":"file_progress","file":"a","current":30,"total":60}` + "\n" +
		`{"type":"file_progress","file":"b","current":20,"total":40}` + "\n"

	n, err := cw.Write([]byte(stream))
	require.NoError(t, err)
	require.Equal(t, len(stream), n)
	require.Equal(t, int64(50), cw.downloadedBytes, "both files' bytes are summed")

	// A file that reaches its total drops out of the in-flight list.
	_, err = cw.Write([]byte(`{"type":"file_progress","file":"a","current":60,"total":60}` + "\n"))
	require.NoError(t, err)
	_, stillA := cw.fileProgress["a"]
	require.False(t, stillA, "a finished file is no longer shown as in flight")
}
