package cmd

import (
	"testing"

	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/pkg/clierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A login that fails has to be visible to a script, not only to a reader:
// --code exists for machines that are driven by scripts.
func TestLoginCmd_FailureIsReportedAsAnExitCode(t *testing.T) {
	cleanDBTables(t)
	setLastCliErr(nil)
	t.Cleanup(func() { setLastCliErr(nil) })

	out, err := captureCombinedOutput(loginCmd(&client.GogClient{TokenURL: "http://127.0.0.1:1"}),
		"--code", "paste the address here")
	require.NoError(t, err)
	assert.Contains(t, out, "Failed to login to GOG.com")

	recorded := getLastCliErr()
	require.NotNil(t, recorded, "a failed login must map to a non-zero exit code")
	assert.Equal(t, clierr.Internal, recorded.Type)
	assert.Equal(t, 1, exitCodeByType[recorded.Type])
}

func TestLoginCmd_SuccessLeavesNoExitError(t *testing.T) {
	cleanDBTables(t)
	setLastCliErr(nil)
	t.Cleanup(func() { setLastCliErr(nil) })

	srv := tokenServer(t, "abc123")

	_, err := captureCombinedOutput(loginCmd(&client.GogClient{TokenURL: srv.URL}), "--code", "abc123")
	require.NoError(t, err)
	assert.Nil(t, getLastCliErr(), "a successful login must exit zero")
}
