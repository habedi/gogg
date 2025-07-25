package main

import (
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestConfigureLogLevelFromEnv_Disabled(t *testing.T) {
	originalLevel := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(originalLevel) })

	testCases := []struct {
		envVal      string
		expectedLvl zerolog.Level
	}{
		{"false", zerolog.Disabled},
		{"0", zerolog.Disabled},
		{"", zerolog.Disabled},
	}

	for _, tc := range testCases {
		os.Setenv("DEBUG_GOGG", tc.envVal)
		configureLogLevelFromEnv()
		if zerolog.GlobalLevel() != tc.expectedLvl {
			t.Errorf("DEBUG_GOGG=%q: expected log level %v, got %v",
				tc.envVal, tc.expectedLvl, zerolog.GlobalLevel())
		}
	}
}

func TestConfigureLogLevelFromEnv_Debug(t *testing.T) {
	originalLevel := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(originalLevel) })

	testCases := []struct {
		envVal      string
		expectedLvl zerolog.Level
	}{
		{"true", zerolog.DebugLevel},
		{"1", zerolog.DebugLevel},
		{"random", zerolog.DebugLevel},
	}

	for _, tc := range testCases {
		os.Setenv("DEBUG_GOGG", tc.envVal)
		configureLogLevelFromEnv()
		if zerolog.GlobalLevel() != tc.expectedLvl {
			t.Errorf("DEBUG_GOGG=%q: expected log level %v, got %v",
				tc.envVal, tc.expectedLvl, zerolog.GlobalLevel())
		}
	}
}

func TestSetupInterruptListener(t *testing.T) {
	stopChan := setupInterruptListener()
	if stopChan == nil {
		t.Error("expected non-nil channel from setupInterruptListener")
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		stopChan <- os.Interrupt
	}()

	select {
	case sig := <-stopChan:
		if sig != os.Interrupt {
			t.Errorf("expected os.Interrupt, got %v", sig)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("did not receive signal on channel")
	}
}

func TestHandleInterrupt(t *testing.T) {
	stopChan := make(chan os.Signal, 1)
	exitCalled := make(chan int, 1)
	var loggedMessage string

	fakeFatalLog := func(msg string) {
		loggedMessage = msg
		// In a real scenario log.Fatal would os.Exit, so we simulate the exit part
		exitCalled <- 1
	}
	// We don't need a fake exit anymore since log.Fatal now handles it.
	fakeExit := func(code int) {
		// This function is no longer called by the modified handleInterrupt
	}

	go handleInterrupt(stopChan, fakeFatalLog, fakeExit)

	stopChan <- os.Interrupt

	select {
	case code := <-exitCalled:
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		expectedMsg := "Interrupt signal received. Exiting..."
		if loggedMessage != expectedMsg {
			t.Errorf("expected log message %q, got %q", expectedMsg, loggedMessage)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("fatal log function was not called on interrupt")
	}
}
