package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/pkg/clierr"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func loginCmd(gogClient *client.GogClient) *cobra.Command {
	var gogUsername, gogPassword string
	var headless bool
	var authCode string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to GOG.com",
		Long: "Login to GOG.com using your username and password.\n\n" +
			"With --code no browser has to be driven, which is the way to log in on a\n" +
			"machine that has none. Open\n\n" +
			"  " + client.GOGLoginURL + "\n\n" +
			"in any browser, log in there, and pass the address you land on (or just the\n" +
			"code from it) to --code.",
		Run: func(cmd *cobra.Command, args []string) {
			if strings.TrimSpace(authCode) != "" {
				if err := gogClient.LoginWithCode(authCode); err != nil {
					reportLoginError(cmd, clierr.New(clierr.Internal, "Failed to login to GOG.com", err))
					return
				}
				cmd.Println("Login was successful.")
				return
			}

			cmd.Println("Please enter your GOG username and password.")
			gogUsername = promptForInput("GOG username: ")
			gogPassword = promptForPassword("GOG password: ")

			if validateCredentials(gogUsername, gogPassword) {
				if err := gogClient.Login(client.GOGLoginURL, gogUsername, gogPassword, headless); err != nil {
					reportLoginError(cmd, clierr.New(clierr.Internal, "Failed to login to GOG.com", err))
					if strings.Contains(err.Error(), "executable found in PATH") {
						cmd.PrintErrln("Hint: Make sure Google Chrome or Chromium is installed and accessible in your system's PATH.")
					}
				} else {
					cmd.Println("Login was successful.")
				}
			} else {
				reportLoginError(cmd, clierr.New(clierr.Validation, "Username and password cannot be empty", nil))
			}
		},
	}

	cmd.Flags().StringVarP(&authCode, "code", "c", "",
		"Authorization code, or the address GOG redirected you to, to log in without driving a browser")
	cmd.Flags().BoolVarP(&headless, "headless", "n", true, "Login in headless mode without showing the browser window? [true, false]")

	return cmd
}

// reportLoginError prints the failure with its cause and records it so the
// process exits non-zero. The cause is what tells the user what to do next:
// which browser could not be driven, or what was wrong with the code.
func reportLoginError(cmd *cobra.Command, e *clierr.Error) {
	cmd.PrintErrln(e.Message)
	if e.Err != nil {
		cmd.PrintErrln("Cause:", e.Err)
	}
	setLastCliErr(e)
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
