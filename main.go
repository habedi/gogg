package main

import (
	"github.com/habedi/template-go-project/cmd"
	"os"
	"os/signal"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// send all logs to stdout
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

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
