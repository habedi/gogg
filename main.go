package main

import (
	"github.com/habedi/gogg/cmd"
	"github.com/rs/zerolog"
	"os"
	"os/signal"

	"github.com/rs/zerolog/log"
)

func main() {

	// If the DEBUG_GOGG environment variable is set, enable debug logging to stdout, otherwise disable logging
	if os.Getenv("DEBUG_GOGG") != "" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.Disabled)
	}

	// This block sets up a go routine to listen for an interrupt signal which will immediately exit the program
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt)
	go listenForInterrupt(stopChan)

	// Execute the CLI program
	cmd.Execute()
}

// listenForInterrupt listens for an interrupt signal and exits the program when it is received
func listenForInterrupt(stopScan chan os.Signal) {
	<-stopScan
	log.Fatal().Msg("Interrupt signal received. Exiting...")
}
