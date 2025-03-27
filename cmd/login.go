package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/habedi/gogg/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// loginCmd creates a new cobra.Command for logging into GOG.com.
// It returns a pointer to the created cobra.Command.
func loginCmd() *cobra.Command {
	var gogUsername, gogPassword string
	var headless bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to GOG.com",
		Long:  "Login to GOG.com using your username and password",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("Please enter your GOG username and password.")
			gogUsername = promptForInput("GOG username: ")
			gogPassword = promptForPassword("GOG password: ")

			if validateCredentials(gogUsername, gogPassword) {
				if err := client.Login(client.GOGLoginURL, gogUsername, gogPassword, headless); err != nil {
					cmd.PrintErrln("Error: Failed to login to GOG.com.")
				} else {
					cmd.Println("Login was successful.")
				}
			} else {
				cmd.PrintErrln("Error: Username and password cannot be empty.")
			}
		},
	}

	// Add flags for login options
	cmd.Flags().BoolVarP(&headless, "headless", "n", true, "Login in headless mode without showing the browser window? [true, false]")

	return cmd
}

// promptForInput prompts the user for input and returns the trimmed string.
// It takes a prompt string as an argument.
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
// It takes a prompt string as an argument.
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
// It takes the username and password strings as arguments and returns a boolean.
func validateCredentials(username, password string) bool {
	return username != "" && password != ""
}
