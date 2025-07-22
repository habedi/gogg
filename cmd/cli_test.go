package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"github.com/spf13/cobra"
)

type mockAuthStorer struct{}

func (m *mockAuthStorer) GetTokenRecord() (*db.Token, error)      { return nil, nil }
func (m *mockAuthStorer) UpsertTokenRecord(token *db.Token) error { return nil }

type mockAuthRefresher struct{}

func (m *mockAuthRefresher) PerformTokenRefresh(refreshToken string) (string, string, int64, error) {
	return "", "", 0, nil
}

func TestCreateRootCmd(t *testing.T) {
	authService := auth.NewService(&mockAuthStorer{}, &mockAuthRefresher{})
	rootCmd := createRootCmd(authService)

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
		authService := auth.NewService(&mockAuthStorer{}, &mockAuthRefresher{})
		rootCmd := createRootCmd(authService)
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
