package main

import (
	"github.com/habedi/gogg/cmd"
	"github.com/rs/zerolog"
	"os"
	"os/signal"
	"strings"

	"github.com/rs/zerolog/log"
)

// main is the entry point of the application.
// It sets up logging based on the DEBUG_GOGG environment variable,
// starts a goroutine to listen for interrupt signals, and executes the main command.
func main() {

	// If the DEBUG_GOGG environment variable is set to false or 0, or not set at all, disable logging, otherwise enable it.
	debugMode := strings.TrimSpace(strings.ToLower(os.Getenv("DEBUG_GOGG")))
	if debugMode == "false" || debugMode == "0" || debugMode == "" {
		zerolog.SetGlobalLevel(zerolog.Disabled)
	} else {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// This block sets up a go routine to listen for an interrupt signal which will immediately exit the program
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt)
	go listenForInterrupt(stopChan)

	// Program entry point
	cmd.Execute()
}

// listenForInterrupt listens for an interrupt signal and exits the program when it is received.
// It takes a channel of os.Signal as a parameter.
func listenForInterrupt(stopScan chan os.Signal) {
	<-stopScan
	log.Fatal().Msg("Interrupt signal received. Exiting...")
}
