package main

import (
	"os"
	"os/signal"
	"strings"

	"github.com/habedi/gogg/cmd"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Info().Msg("Gogg starting up")
	configureLogLevelFromEnv()

	stopChan := setupInterruptListener()
	go handleInterrupt(stopChan,
		func(msg string) { log.Warn().Msg(msg) }, // avoid log.Fatal to keep control flow explicit
		func(code int) {
			log.Info().Msg("Shutdown initiated")
			// graceful cleanup
			db.Shutdown()
			log.Info().Msg("Shutdown complete")
			os.Exit(code)
		},
	)
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

func handleInterrupt(stopChan chan os.Signal, logFunc func(string), exitFunc func(int)) {
	<-stopChan
	logFunc("Interrupt signal received. Exiting...")
	exitFunc(1)
}

func execute() {
	cmd.Execute()
}
