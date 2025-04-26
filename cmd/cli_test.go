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

func TestCreateRootCmd(t *testing.T) {
	rootCmd := createRootCmd()
	if rootCmd.Use != "gogg" {
		t.Errorf("expected root command use to be 'gogg', got: %s", rootCmd.Use)
	}

	subCommands := rootCmd.Commands()
	if len(subCommands) == 0 {
		t.Error("expected root command to have subcommands, got none")
	}

	for _, cmd := range subCommands {
		if cmd.Use == "help" {
			t.Error("expected help command to be replaced, but found a subcommand with use 'help'")
		}
	}
}

func TestInitializeAndCloseDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	db.Path = filepath.Join(tmpDir, "games.db")
	initializeDatabase()
	closeDatabase()
}

func TestExecuteFailure(t *testing.T) {
	if os.Getenv("TEST_EXECUTE_FAILURE") == "1" {
		rootCmd := createRootCmd()
		rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
			return errors.New("dummy failure")
		}
		if err := rootCmd.Execute(); err != nil {
			os.Exit(1)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestExecuteFailure")
	cmd.Env = append(os.Environ(), "TEST_EXECUTE_FAILURE=1")
	err := cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if exitError.ExitCode() != 1 {
			t.Fatalf("expected exit code 1, got %d", exitError.ExitCode())
		}
	}
}
