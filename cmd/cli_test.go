package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/habedi/gogg/db"
	"github.com/spf13/cobra"
)

// TestCreateRootCmd checks that createRootCmd returns a root command
// with the expected use string, subcommands, and a replaced help command.
func TestCreateRootCmd(t *testing.T) {
	rootCmd := createRootCmd()
	if rootCmd.Use != "gogg" {
		t.Errorf("expected root command use to be 'gogg', got: %s", rootCmd.Use)
	}

	subCommands := rootCmd.Commands()
	if len(subCommands) == 0 {
		t.Error("expected root command to have subcommands, got none")
	}

	// Verify that the default help command is replaced (i.e. no subcommand with Use "help")
	for _, cmd := range subCommands {
		if cmd.Use == "help" {
			t.Error("expected help command to be replaced, but found a subcommand with use 'help'")
		}
	}
}

// TestInitializeAndCloseDatabase sets a temporary DB path and calls
// initializeDatabase and closeDatabase. If no os.Exit occurs, the test passes.
func TestInitializeAndCloseDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	db.Path = filepath.Join(tmpDir, "games.db")
	// initializeDatabase should succeed (using the temporary path)
	initializeDatabase()
	// closeDatabase should also succeed
	closeDatabase()
}

// TestExecuteFailure runs a subprocess where the root command's RunE is overridden
// to always return an error. In that case Execute (or a call to Execute-like behavior)
// should call os.Exit(1). We capture the exit code via os/exec.
func TestExecuteFailure(t *testing.T) {
	// If this is the child process, override the command to simulate failure.
	if os.Getenv("TEST_EXECUTE_FAILURE") == "1" {
		// Create a root command and override its RunE so that it returns an error.
		rootCmd := createRootCmd()
		rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
			return errors.New("dummy failure")
		}
		// Execute the command. If an error is returned, exit with 1.
		if err := rootCmd.Execute(); err != nil {
			os.Exit(1)
		}
		return
	}

	// In the parent process, run this test in a subprocess.
	cmd := exec.Command(os.Args[0], "-test.run=TestExecuteFailure")
	cmd.Env = append(os.Environ(), "TEST_EXECUTE_FAILURE=1")
	err := cmd.Run()
	if exitError, ok := err.(*exec.ExitError); ok {
		if exitError.ExitCode() != 1 {
			t.Fatalf("expected exit code 1, got %d", exitError.ExitCode())
		}
	} else if err == nil {
		t.Fatalf("expected an exit error, but command succeeded")
	} else {
		t.Fatalf("unexpected error: %v", err)
	}
}
