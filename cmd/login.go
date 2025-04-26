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

	cmd.Flags().BoolVarP(&headless, "headless", "n", true, "Login in headless mode without showing the browser window? [true, false]")

	return cmd
}

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

func promptForPassword(prompt string) string {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println("Error: Failed to read password.")
		os.Exit(1)
	}
	fmt.Println()
	return strings.TrimSpace(string(password))
}

func validateCredentials(username, password string) bool {
	return username != "" && password != ""
}
