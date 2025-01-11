package cmd

import (
	"bufio"
	"fmt"
	"github.com/habedi/gogg/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"os"
	"strings"
)

// initCmd initializes Gogg for first-time use by saving the user credentials in the internal database.
func initCmd() *cobra.Command {
	var gogUsername, gogPassword string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Gogg for first-time use",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("Please enter your GOG username and password.")
			gogUsername = promptForInput("GOG username: ")
			gogPassword = promptForPassword("GOG password: ")

			if validateCredentials(gogUsername, gogPassword) {
				if err := client.SaveUserCredentials(gogUsername, gogPassword); err != nil {
					cmd.PrintErrln("Error: Failed to save the credentials.")
				} else {
					cmd.Println("Credentials saved successfully.")
				}
			} else {
				cmd.PrintErrln("Error: Username and password cannot be empty.")
			}
		},
	}

	return cmd
}

// promptForInput prompts the user for input and returns the trimmed string.
func promptForInput(prompt string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error: Failed to read input.")
		os.Exit(1)
	}
	return strings.TrimSpace(input)
}

// promptForPassword prompts the user for a password securely and returns the trimmed string.
func promptForPassword(prompt string) string {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println("Error: Failed to read password.")
		os.Exit(1)
	}
	fmt.Println() // Print a newline for better formatting
	return strings.TrimSpace(string(password))
}

// validateCredentials checks if the username and password are not empty.
func validateCredentials(username, password string) bool {
	return username != "" && password != ""
}
