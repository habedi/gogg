package main

import (
	"os"
	"os/signal"
	"strings"

	"github.com/habedi/gogg/cmd"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	configureLogLevelFromEnv()

	stopChan := setupInterruptListener()
	go handleInterrupt(stopChan, func(msg string) {
		log.Fatal().Msg(msg)
	}, os.Exit)

	execute()
}

// configureLogLevelFromEnv reads the DEBUG_GOGG env variable and sets the global log level.
func configureLogLevelFromEnv() {
	debugMode := strings.TrimSpace(strings.ToLower(os.Getenv("DEBUG_GOGG")))
	switch debugMode {
	case "false", "0", "":
		zerolog.SetGlobalLevel(zerolog.Disabled)
	default:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}

// setupInterruptListener creates and returns a channel for interrupt signals.
func setupInterruptListener() chan os.Signal {
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt)
	return stopChan
}

// handleInterrupt listens for an interrupt signal on stopChan.
// When received, it calls the injected logging function (fatalLog)
// and then calls the injected exit function (exitFunc).
func handleInterrupt(stopChan chan os.Signal, fatalLog func(string), exitFunc func(int)) {
	<-stopChan
	fatalLog("Interrupt signal received. Exiting...")
	exitFunc(1)
}

// execute is the main entry point for the application logic.
func execute() {
	cmd.Execute()
}
