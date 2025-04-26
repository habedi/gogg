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

func configureLogLevelFromEnv() {
	debugMode := strings.TrimSpace(strings.ToLower(os.Getenv("DEBUG_GOGG")))
	switch debugMode {
	case "false", "0", "":
		zerolog.SetGlobalLevel(zerolog.Disabled)
	default:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}

func setupInterruptListener() chan os.Signal {
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt)
	return stopChan
}

func handleInterrupt(stopChan chan os.Signal, fatalLog func(string), exitFunc func(int)) {
	<-stopChan
	fatalLog("Interrupt signal received. Exiting...")
	exitFunc(1)
}

func execute() {
	cmd.Execute()
}
